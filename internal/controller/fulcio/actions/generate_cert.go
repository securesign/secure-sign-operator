package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	FulcioCALabel = constants.LabelNamespace + "/fulcio_v1.crt.pem"
)

func NewHandleCertAction() action.Action[*v1alpha1.Fulcio] {
	return &handleCert{}
}

type handleCert struct {
	action.BaseAction
}

func (g handleCert) Name() string {
	return "handle-cert"
}

func (g handleCert) CanHandle(_ context.Context, instance *v1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)

	switch {
	case c == nil:
		return false
	case !(c.Reason == constants.Pending || c.Reason == constants.Ready):
		return false
	case !meta.IsStatusConditionTrue(instance.GetConditions(), CertCondition):
		return true
	default:
		return !equality.Semantic.DeepDerivative(instance.Spec.Certificate, *instance.Status.Certificate)
	}

}

func (g handleCert) Handle(ctx context.Context, instance *v1alpha1.Fulcio) *action.Result {
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Pending,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   CertCondition,
			Status: metav1.ConditionFalse,
			Reason: constants.Creating,
		})
		return g.StatusUpdate(ctx, instance)
	}

	if instance.Spec.Certificate.PrivateKeyRef == nil && instance.Spec.Certificate.CARef != nil {
		err := fmt.Errorf("missing private key for CA certificate")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CertCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return g.FailedWithStatusUpdate(ctx, err, instance)
	}

	instance.Status.Certificate = instance.Spec.Certificate.DeepCopy()
	if err := g.calculateHostname(ctx, instance); err != nil {
		return g.Failed(err)
	}

	//Check if a secret for the  fulcio cert already exists and validate
	partialSecret, err := k8sutils.FindSecret(ctx, g.Client, instance.Namespace, FulcioCALabel)
	if err != nil && !apierrors.IsNotFound(err) {
		g.Logger.Error(err, "problem with finding secret", "namespace", instance.Namespace)
	}

	if partialSecret != nil {
		if equality.Semantic.DeepDerivative(g.certMatchingAnnotations(instance), partialSecret.GetAnnotations()) {
			// certificate is valid
			if secret, err := k8sutils.GetSecret(g.Client, partialSecret.Namespace, partialSecret.Name); err != nil {
				return g.Failed(fmt.Errorf("can't load CA secret %w", err))
			} else {
				g.alignStatusFields(secret, instance)
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   CertCondition,
					Status: metav1.ConditionTrue,
					Reason: "Resolved",
				})
				return g.StatusUpdate(ctx, instance)
			}
		}

		// invalidate certificate
		if err := constants.RemoveLabel(ctx, partialSecret, g.Client, FulcioCALabel); err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    CertCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			return g.FailedWithStatusUpdate(ctx, err, instance)
		}
		message := fmt.Sprintf("Removed '%s' label from %s secret", FulcioCALabel, partialSecret.Name)
		g.Recorder.Event(instance, v1.EventTypeNormal, "CertificateSecretLabelRemoved", message)
		g.Logger.Info(message)
	}

	cert, err := g.setupCert(instance)
	if err != nil {
		g.Logger.Error(err, "error resolving certificate for fulcio")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CertCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "Resolving keys",
		})
		g.StatusUpdate(ctx, instance)
		// swallow error and retry
		return g.Requeue()
	}

	secretLabels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
	secretLabels[FulcioCALabel] = "cert"
	newCert := k8sutils.CreateImmutableSecret(fmt.Sprintf("fulcio-cert-%s", instance.Name), instance.Namespace, cert.ToData(), secretLabels)
	newCert.Annotations = g.certMatchingAnnotations(instance)

	if _, err := g.Ensure(ctx, newCert); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CertCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return g.FailedWithStatusUpdate(ctx, err, instance)
	}
	g.Recorder.Event(instance, v1.EventTypeNormal, "FulcioCertUpdated", "Fulcio certificate secret updated")

	g.alignStatusFields(newCert, instance)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   CertCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	})
	return g.StatusUpdate(ctx, instance)
}

func (g handleCert) setupCert(instance *v1alpha1.Fulcio) (*utils.FulcioCertConfig, error) {
	config := &utils.FulcioCertConfig{
		OrganizationEmail: instance.Status.Certificate.OrganizationEmail,
		OrganizationName:  instance.Status.Certificate.OrganizationName,
		CommonName:        instance.Status.Certificate.CommonName,
	}

	if ref := instance.Status.Certificate.PrivateKeyPasswordRef; ref != nil {
		password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKeyPassword = password
	} else if instance.Status.Certificate.PrivateKeyRef == nil {
		config.PrivateKeyPassword = common.GeneratePassword(8)
	}
	if ref := instance.Status.Certificate.PrivateKeyRef; ref != nil {
		key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = key
	} else {
		key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
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

	if ref := instance.Status.Certificate.CARef; ref != nil {
		key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
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

func (g handleCert) alignStatusFields(secret *v1.Secret, instance *v1alpha1.Fulcio) {
	if instance.Status.Certificate.PrivateKeyRef == nil {
		instance.Status.Certificate.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			Key: "private",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}

	if val, ok := secret.Data["password"]; instance.Spec.Certificate.PrivateKeyPasswordRef == nil && ok && len(val) > 0 {
		instance.Status.Certificate.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
			Key: "password",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}

	if instance.Spec.Certificate.CARef == nil {
		instance.Status.Certificate.CARef = &v1alpha1.SecretKeySelector{
			Key: "cert",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}
}

func (g handleCert) calculateHostname(ctx context.Context, instance *v1alpha1.Fulcio) error {
	var err error
	if instance.Status.Certificate.CommonName != "" {
		return nil
	}

	if !instance.Spec.ExternalAccess.Enabled {
		instance.Status.Certificate.CommonName = fmt.Sprintf("%s.%s.svc.local", DeploymentName, instance.Namespace)
		return nil
	}

	if instance.Spec.ExternalAccess.Host != "" {
		instance.Status.Certificate.CommonName = instance.Spec.ExternalAccess.Host
		return nil
	}

	instance.Spec.ExternalAccess.Host, err = k8sutils.CalculateHostname(ctx, g.Client, DeploymentName, instance.Namespace)

	return err
}
func (g handleCert) certMatchingAnnotations(instance *v1alpha1.Fulcio) map[string]string {
	m := map[string]string{
		constants.LabelNamespace + "/commonName":        instance.Status.Certificate.CommonName,
		constants.LabelNamespace + "/organizationEmail": instance.Status.Certificate.OrganizationEmail,
		constants.LabelNamespace + "/organizationName":  instance.Status.Certificate.OrganizationName,
	}

	if instance.Spec.Certificate.PrivateKeyRef != nil {
		// private key is user specified - it does matter
		m[constants.LabelNamespace+"/privateKeyRef"] = instance.Spec.Certificate.PrivateKeyRef.Name
	}
	if instance.Spec.Certificate.PrivateKeyPasswordRef != nil {
		// private key is user specified - it does matter
		m[constants.LabelNamespace+"/passwordKeyRef"] = instance.Spec.Certificate.PrivateKeyPasswordRef.Name
	}

	return m
}
