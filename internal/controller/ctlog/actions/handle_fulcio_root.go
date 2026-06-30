package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
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
// Spec.RootCertificates empty → autodiscovery: operator resolves certs from Fulcio CR status.
// Spec.RootCertificates set   → user-provided: operator uses the explicit refs from spec.
func (g handleFulcioCert) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	switch {
	case c == nil:
		return false
	case state.FromReason(c.Reason) < state.Creating:
		return false
	case len(instance.Status.RootCertificates) == 0:
		// No certs resolved yet — initial resolution needed.
		return true
	case len(instance.Spec.RootCertificates) == 0:
		// Autodiscovery: always re-run Handle so it can compare the provisioned cert
		// against Fulcio CR's current status and detect rotation.
		// Handle itself short-circuits with Continue() when content is unchanged.
		return true
	default:
		// User-provided: only re-run when spec refs differ from status refs.
		return !equality.Semantic.DeepDerivative(instance.Spec.RootCertificates, instance.Status.RootCertificates)
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

	if len(instance.Spec.RootCertificates) == 0 {
		cert, err := g.discoverFulcioRootCert(ctx, instance.Namespace)
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
			existing, readErr := k8sutils.GetSecretData(g.Client, instance.Namespace, &instance.Status.RootCertificates[0])
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
		instance.Status.RootCertificates = instance.Spec.RootCertificates
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
		Reason: "Resolved",
	})
	return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
}

func (g handleFulcioCert) discoverFulcioRootCert(ctx context.Context, namespace string) ([]byte, error) {
	item, err := trustmaterial.FindReadyInstance(ctx, g.Client, namespace, &rhtasv1.FulcioList{})
	if err != nil {
		if errors.Is(err, trustmaterial.ErrNoReadyInstance) {
			return nil, fmt.Errorf("no ready fulcio instance found")
		}
		return nil, err
	}
	fulcio, ok := item.(*rhtasv1.Fulcio)
	if !ok {
		return nil, fmt.Errorf("unexpected type %T for ready Fulcio instance", item)
	}
	if fulcio.Status.CertificateChain == "" {
		return nil, fmt.Errorf("fulcio root certificate not yet available")
	}
	return []byte(fulcio.Status.CertificateChain), nil
}
