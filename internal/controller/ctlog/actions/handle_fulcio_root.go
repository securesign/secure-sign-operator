package actions

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	fulcioRootSecretFormat = "ctlog-fulcio-root-%s"
	fulcioRootCertKey      = "cert"
)

func NewHandleFulcioCertAction() action.Action[*rhtasv1.CTlog] {
	return &handleFulcioCert{}
}

type handleFulcioCert struct {
	action.BaseAction
}

func (g handleFulcioCert) Name() string {
	return "handle-fulcio-cert"
}

// CanHandle gates on the component's readiness state and cert resolution status.
//
// Active log Roots empty → autodiscovery: operator resolves certs from Fulcio CR status.
// Active log Roots set   → user-provided: operator uses the explicit refs from spec.
func (g handleFulcioCert) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	activeLog := utils.ActiveLog(instance.Spec.Logs)
	switch {
	case c == nil:
		return false
	case state.FromReason(c.Reason) < state.Creating:
		return false
	case len(instance.Status.RootCertificates) == 0:
		return true
	case activeLog == nil || len(activeLog.Roots) == 0:
		return true
	default:
		return !equality.Semantic.DeepDerivative(activeLog.Roots, instance.Status.RootCertificates)
	}
}

func (g handleFulcioCert) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	previouslyResolved := len(instance.Status.RootCertificates) > 0

	if !previouslyResolved && state.FromInstance(instance, constants.ReadyCondition) != state.Creating {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			ObservedGeneration: instance.Generation,
		})
		return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
	}

	activeLog := utils.ActiveLog(instance.Spec.Logs)
	userProvidedRoots := activeLog != nil && len(activeLog.Roots) > 0

	if !userProvidedRoots {
		cert, err := g.discoverFulcioRootCert(ctx, instance)
		if err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    CertCondition,
				Status:  metav1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
			if _, err := g.PersistStatus(ctx, instance); err != nil {
				return g.Error(ctx, err, instance)
			}
			return g.RequeueAfter(5 * time.Second)
		}

		signingCert, err := trustmaterial.ExtractSigningCert(cert)
		if err != nil {
			return g.Error(ctx, fmt.Errorf("extracting signing cert from Fulcio trust bundle: %w", err), instance)
		}

		if previouslyResolved {
			existing, readErr := k8sutils.GetSecretData(ctx, g.Client, instance.Namespace, &instance.Status.RootCertificates[0])
			if readErr == nil && bytes.Equal(existing, signingCert) {
				return g.Continue()
			}
		}

		secretName := fmt.Sprintf(fulcioRootSecretFormat, instance.Name)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: instance.Namespace,
			},
		}
		componentLabels := labels.ForComponent(ComponentName, instance.Name)
		if _, err := k8sutils.CreateOrUpdate(ctx, g.Client, secret,
			ensure.ControllerReference[*corev1.Secret](instance, g.Client),
			ensure.Labels[*corev1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
			k8sutils.EnsureSecretData(false, map[string][]byte{fulcioRootCertKey: signingCert}),
		); err != nil {
			return g.Error(ctx, err, instance)
		}

		sks := rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: secretName},
			Key:                  fulcioRootCertKey,
		}
		if previouslyResolved {
			g.Recorder.Eventf(instance, nil, corev1.EventTypeNormal, "FulcioCertRotated", "Rotated", "Fulcio root certificate rotated — updating CTlog config")
		} else {
			g.Recorder.Eventf(instance, nil, corev1.EventTypeNormal, "FulcioCertDiscovered", "Discovered", "Fulcio root certificate resolved from Fulcio CR status")
		}
		instance.Status.RootCertificates = []rhtasv1.SecretKeySelector{sks}
	} else {
		instance.Status.RootCertificates = activeLog.Roots
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ConfigCondition,
		Status:  metav1.ConditionFalse,
		Reason:  FulcioReason,
		Message: "Fulcio certificate changed",
	})

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   CertCondition,
		Status: metav1.ConditionTrue,
		Reason: constants.ReasonResolved,
	})
	return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
}

func (g handleFulcioCert) discoverFulcioRootCert(ctx context.Context, instance *rhtasv1.CTlog) ([]byte, error) {
	fulcio := &rhtasv1.Fulcio{}
	if err := serviceresolver.PopulateInstance(ctx, g.Client, instance.Spec.Fulcio, instance.Namespace, fulcio); err != nil {
		return nil, fmt.Errorf("resolving Fulcio instance: %w", err)
	}
	if fulcio.Status.CertificateChain == "" {
		return nil, fmt.Errorf("fulcio root certificate not yet available")
	}
	return []byte(fulcio.Status.CertificateChain), nil
}
