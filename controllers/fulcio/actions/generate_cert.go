package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"maps"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/fulcio/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const SecretNameFormat = "fulcio-%s-cert"

func NewGenerateCertAction() action.Action[v1alpha1.Fulcio] {
	return &generateCert{}
}

type generateCert struct {
	action.BaseAction
}

func (g generateCert) Name() string {
	return "generate-cert"
}

func (g generateCert) CanHandle(instance *v1alpha1.Fulcio) bool {
	return instance.Status.Phase == v1alpha1.PhaseNone || instance.Status.Phase == v1alpha1.PhasePending
}

func (g generateCert) Handle(ctx context.Context, instance *v1alpha1.Fulcio) *action.Result {
	if instance.Status.Phase == v1alpha1.PhaseNone {
		instance.Status.Phase = v1alpha1.PhasePending
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   "FulcioCert",
			Status: metav1.ConditionUnknown,
			Reason: "Resolving",
		})
		return g.StatusUpdate(ctx, instance)
	}

	if instance.Spec.Certificate.PrivateKeyRef == nil && instance.Spec.Certificate.CARef != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(v1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: "There is set CARef field but there is no PrivateKeyRef secret attached.",
		})
		return g.FailedWithStatusUpdate(ctx, fmt.Errorf("missing private key for CA certificate"), instance)
	}

	secretName := fmt.Sprintf(SecretNameFormat, instance.Name)
	var updated bool

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	// TODO: tturek
	secretLabels := map[string]string{
		constants.TufLabelNamespace + "/fulcio_v1.crt.pem": "cert",
	}
	maps.Copy(secretLabels, labels)

	config, err := g.setupCert(instance)
	if err != nil {
		if !meta.IsStatusConditionFalse(instance.Status.Conditions, "FulcioCert") {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    "FulcioCert",
				Status:  metav1.ConditionFalse,
				Reason:  "Failure",
				Message: err.Error(),
			})
			return g.StatusUpdate(ctx, instance)
		}

		// swallow error and retry
		return g.Requeue()
	}

	secret := k8sutils.CreateSecret(secretName, instance.Namespace, config.ToMap(), secretLabels)
	if err = controllerutil.SetControllerReference(instance, secret, g.Client.Scheme()); err != nil {
		return g.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}
	if updated, err = g.Ensure(ctx, secret); err != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(v1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return g.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create secret: %w", err), instance)
	}

	if updated && (instance.Spec.Certificate.PrivateKeyRef == nil ||
		(instance.Spec.Certificate.PrivateKeyPasswordRef == nil && len(config.PrivateKeyPassword) > 0) ||
		instance.Spec.Certificate.CARef == nil) {
		if instance.Spec.Certificate.PrivateKeyRef == nil {
			instance.Spec.Certificate.PrivateKeyRef = &v1alpha1.SecretKeySelector{
				Key: "private",
				LocalObjectReference: v1.LocalObjectReference{
					Name: secretName,
				},
			}
		}

		if instance.Spec.Certificate.PrivateKeyPasswordRef == nil && len(config.PrivateKeyPassword) > 0 {
			instance.Spec.Certificate.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
				Key: "password",
				LocalObjectReference: v1.LocalObjectReference{
					Name: secretName,
				},
			}
		}

		if instance.Spec.Certificate.CARef == nil {
			instance.Spec.Certificate.CARef = &v1alpha1.SecretKeySelector{
				Key: "cert",
				LocalObjectReference: v1.LocalObjectReference{
					Name: secretName,
				},
			}
		}

		return g.Update(ctx, instance)
	}
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   "FulcioCert",
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: string(v1alpha1.PhaseReady),
		Status: metav1.ConditionTrue, Reason: string(v1alpha1.PhaseCreating)})
	instance.Status.Phase = v1alpha1.PhaseCreating
	return g.StatusUpdate(ctx, instance)
}

func (g generateCert) setupCert(instance *v1alpha1.Fulcio) (*utils.FulcioCertConfig, error) {
	config := &utils.FulcioCertConfig{}

	if ref := instance.Spec.Certificate.PrivateKeyPasswordRef; ref != nil {
		password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKeyPassword = password
	} else if instance.Spec.Certificate.PrivateKeyRef == nil {
		config.PrivateKeyPassword = common.GeneratePassword(8)
	}
	if ref := instance.Spec.Certificate.PrivateKeyRef; ref != nil {
		key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = key
	} else {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}

		pemKey, err := utils.CreateCAKey(key, config.PrivateKeyPassword)
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

	if ref := instance.Spec.Certificate.CARef; ref != nil {
		key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.RootCert = key
	} else {
		rootCert, err := utils.CreateFulcioCA(config, instance)
		if err != nil {
			return nil, err
		}
		config.RootCert = rootCert
	}

	return config, nil
}
