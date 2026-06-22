package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"maps"
	"slices"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	FulcioCALabel = labels.LabelNamespace + "/fulcio_v1.crt.pem"
)

var managedAnnotations = []string{
	labels.LabelNamespace + "/commonName",
	labels.LabelNamespace + "/organizationEmail",
	labels.LabelNamespace + "/organizationName",
	labels.LabelNamespace + "/privateKeyRef",
	labels.LabelNamespace + "/passwordKeyRef",
}

func NewHandleCertAction() action.Action[*rhtasv1.Fulcio] {
	return &handleCert{}
}

type handleCert struct {
	action.BaseAction
}

func (g handleCert) Name() string {
	return "handle-cert"
}

func (g handleCert) CanHandle(_ context.Context, instance *rhtasv1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)

	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Pending:
		return false
	case instance.Status.Certificate == nil:
		return true
	default:
		// CertCondition is managed exclusively by this action.
		cc := meta.FindStatusCondition(instance.GetConditions(), CertCondition)
		return cc == nil || cc.Status != metav1.ConditionTrue || instance.Generation != cc.ObservedGeneration
	}

}

func (g handleCert) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	if instance.Spec.Certificate.PrivateKeyRef == nil && instance.Spec.Certificate.CARef != nil {
		err := reconcile.TerminalError(fmt.Errorf("missing private key for CA certificate"))
		return g.Error(ctx, err, instance, metav1.Condition{
			Type:               CertCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
	}

	commonName, err := g.resolveCommonName(ctx, instance)
	if err != nil {
		return g.Error(ctx, err, instance)
	}

	// Check if the resolved secret still matches the current spec.
	partialSecret, err := kubernetes.FindSecret(ctx, g.Client, instance.Namespace, FulcioCALabel)
	if client.IgnoreNotFound(err) != nil {
		g.Logger.Error(err, "problem with finding secret")
	}

	if partialSecret != nil {
		if equality.Semantic.DeepDerivative(g.certMatchingAnnotations(commonName, instance), partialSecret.GetAnnotations()) {
			// certificate is valid
			if secret, err := kubernetes.GetSecret(g.Client, partialSecret.Namespace, partialSecret.Name); err != nil {
				return g.Error(ctx, fmt.Errorf("can't load CA secret %w", err), instance)
			} else {
				g.alignStatusFields(secret, instance)
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               CertCondition,
					Status:             metav1.ConditionTrue,
					Reason:             "Resolved", //nolint:goconst
					ObservedGeneration: instance.Generation,
				})
				return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
			}
		}

		// invalidate certificate
		if err := labels.Remove(ctx, partialSecret, g.Client, FulcioCALabel); err != nil {
			return g.Error(ctx, err, instance, metav1.Condition{
				Type:               CertCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
		}
		message := fmt.Sprintf("Removed '%s' label from %s secret", FulcioCALabel, partialSecret.Name)
		g.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "CertificateSecretLabelRemoved", "LabelRemoved", message)
		g.Logger.Info(message)
	}

	// Spec changed or first run — invalidate and transition to Pending.
	instance.Status.Certificate = &rhtasv1.FulcioCertStatus{
		PrivateKeyRef:         instance.Spec.Certificate.PrivateKeyRef.DeepCopy(),
		PrivateKeyPasswordRef: instance.Spec.Certificate.PrivateKeyPasswordRef.DeepCopy(), //nolint:staticcheck
		CARef:                 instance.Spec.Certificate.CARef.DeepCopy(),
		CommonName:            commonName,
	}
	if state.FromInstance(instance, constants.ReadyCondition) != state.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Pending.String(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               CertCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			ObservedGeneration: instance.Generation,
		})
		return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
	}

	cert, err := g.setupCert(commonName, instance)
	if err != nil {
		g.Logger.Error(err, "error resolving certificate for fulcio")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               CertCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Pending.String(),
			Message:            "Resolving keys",
			ObservedGeneration: instance.Generation,
		})
		if _, err := g.PersistStatus(ctx, instance); err != nil {
			return g.Error(ctx, err, instance)
		}
		// swallow error and retry
		return g.RequeueAfter(5 * time.Second)
	}

	componentLabels := labels.For(ComponentName, DeploymentName, instance.Name)
	keyLabels := map[string]string{FulcioCALabel: constants.KeyCert}
	annotations := g.certMatchingAnnotations(commonName, instance)

	newCert := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("fulcio-cert-%s", instance.Name),
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, g.Client,
		newCert,
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(keyLabels)), keyLabels),
		ensure.Annotations[*v1.Secret](managedAnnotations, annotations),
		kubernetes.EnsureSecretData(true, cert.ToData()),
	); err != nil {
		return g.Error(ctx, fmt.Errorf("can't generate certificate secret: %w", err), instance,
			metav1.Condition{
				Type:               CertCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
	}

	g.Recorder.Eventf(instance, newCert, v1.EventTypeNormal, "FulcioCertUpdated", "Updated", "Fulcio certificate secret updated")

	g.alignStatusFields(newCert, instance)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               CertCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "Resolved",
		ObservedGeneration: instance.Generation,
	})
	return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
}

func (g handleCert) setupCert(commonName string, instance *rhtasv1.Fulcio) (*utils.FulcioCertConfig, error) {
	config := &utils.FulcioCertConfig{
		OrganizationEmail: instance.Spec.Certificate.OrganizationEmail,
		OrganizationName:  instance.Spec.Certificate.OrganizationName,
		CommonName:        commonName,
	}

	if ref := instance.Status.Certificate.PrivateKeyPasswordRef; ref != nil {
		password, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKeyPassword = password
	}
	if ref := instance.Status.Certificate.PrivateKeyRef; ref != nil {
		key, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = key
	} else {
		key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, err
		}

		pemKey, err := utils.CreateCAKey(key)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = pemKey

		pemPubKey, err := utils.CreateCAPub(key.Public())
		if err != nil {
			return nil, err
		}
		config.PublicKey = pemPubKey
	}

	if ref := instance.Status.Certificate.CARef; ref != nil {
		key, err := kubernetes.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.RootCert = key
	} else {
		rootCert, err := utils.CreateFulcioCA(config)
		if err != nil {
			return nil, err
		}
		config.RootCert = rootCert
	}

	return config, nil
}

func (g handleCert) alignStatusFields(secret *v1.Secret, instance *rhtasv1.Fulcio) {
	if instance.Status.Certificate == nil {
		instance.Status.Certificate = &rhtasv1.FulcioCertStatus{
			PrivateKeyRef:         instance.Spec.Certificate.PrivateKeyRef.DeepCopy(),
			PrivateKeyPasswordRef: instance.Spec.Certificate.PrivateKeyPasswordRef.DeepCopy(), //nolint:staticcheck
			CARef:                 instance.Spec.Certificate.CARef.DeepCopy(),
		}
	}
	if instance.Status.Certificate.CommonName == "" {
		if cn, ok := secret.Annotations[labels.LabelNamespace+"/commonName"]; ok {
			instance.Status.Certificate.CommonName = cn
		}
	}
	if instance.Status.Certificate.PrivateKeyRef == nil {
		instance.Status.Certificate.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key: constants.KeyPrivate,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}

	if val, ok := secret.Data[constants.KeyPassword]; instance.Spec.Certificate.PrivateKeyPasswordRef == nil && ok && len(val) > 0 {
		instance.Status.Certificate.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{
			Key: constants.KeyPassword,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}

	if instance.Spec.Certificate.CARef == nil {
		instance.Status.Certificate.CARef = &rhtasv1.SecretKeySelector{
			Key: constants.KeyCert,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}
}

func (g handleCert) resolveCommonName(ctx context.Context, instance *rhtasv1.Fulcio) (string, error) {
	if instance.Spec.Certificate.CommonName != "" {
		return instance.Spec.Certificate.CommonName, nil
	}
	if instance.Status.Certificate != nil && instance.Status.Certificate.CommonName != "" {
		return instance.Status.Certificate.CommonName, nil
	}
	if !instance.Spec.ExternalAccess.Enabled {
		return fmt.Sprintf("%s.%s.svc.local", DeploymentName, instance.Namespace), nil
	}
	if instance.Spec.ExternalAccess.Host != "" {
		return instance.Spec.ExternalAccess.Host, nil
	}
	return kubernetes.CalculateHostname(ctx, g.Client, DeploymentName, instance.Namespace)
}

func (g handleCert) certMatchingAnnotations(commonName string, instance *rhtasv1.Fulcio) map[string]string {
	m := map[string]string{
		labels.LabelNamespace + "/commonName":        commonName,
		labels.LabelNamespace + "/organizationEmail": instance.Spec.Certificate.OrganizationEmail,
		labels.LabelNamespace + "/organizationName":  instance.Spec.Certificate.OrganizationName,
	}

	if instance.Spec.Certificate.PrivateKeyRef != nil {
		// private key is user specified - it does matter
		m[labels.LabelNamespace+"/privateKeyRef"] = instance.Spec.Certificate.PrivateKeyRef.Name
	}
	if instance.Spec.Certificate.PrivateKeyPasswordRef != nil {
		// private key is user specified - it does matter
		m[labels.LabelNamespace+"/passwordKeyRef"] = instance.Spec.Certificate.PrivateKeyPasswordRef.Name
	}
	return m
}
