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
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	FulcioCALabel = constants.LabelNamespace + "/fulcio_v1.crt.pem"
)

func NewHandleCertAction() action.Action[v1alpha1.Fulcio] {
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
	return (c.Reason == constants.Pending || c.Reason == constants.Ready) && (instance.Status.Certificate == nil ||
		!equality.Semantic.DeepDerivative(instance.Spec.Certificate, *instance.Status.Certificate))
}

func (g handleCert) Handle(ctx context.Context, instance *v1alpha1.Fulcio) *action.Result {
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Pending,
		},
		)
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
	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	secretLabels := map[string]string{
		FulcioCALabel: "cert",
	}
	maps.Copy(secretLabels, labels)

	cert, err := g.setupCert(ctx, instance)
	if err != nil {
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

	newCert := k8sutils.CreateImmutableSecret(fmt.Sprintf("fulcio-cert-%s", instance.Name), instance.Namespace, cert.ToMap(), secretLabels)
	if err = controllerutil.SetControllerReference(instance, newCert, g.Client.Scheme()); err != nil {
		return g.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}
	// ensure that only new key is exposed
	if err = g.Client.DeleteAllOf(ctx, &v1.Secret{}, client.InNamespace(instance.Namespace), client.MatchingLabels(constants.LabelsFor(ComponentName, DeploymentName, instance.Name)), client.HasLabels{FulcioCALabel}); err != nil {
		return g.Failed(err)
	}
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

	if instance.Status.Certificate == nil {
		instance.Status.Certificate = new(v1alpha1.FulcioCert)
	}

	instance.Spec.Certificate.DeepCopyInto(instance.Status.Certificate)
	if instance.Spec.Certificate.PrivateKeyRef == nil {
		instance.Status.Certificate.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			Key: "private",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: newCert.Name,
			},
		}
	}

	if instance.Spec.Certificate.PrivateKeyPasswordRef == nil && len(cert.PrivateKeyPassword) > 0 {
		instance.Status.Certificate.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
			Key: "password",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: newCert.Name,
			},
		}
	}

	if instance.Spec.Certificate.CARef == nil {
		instance.Status.Certificate.CARef = &v1alpha1.SecretKeySelector{
			Key: "cert",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: newCert.Name,
			},
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   CertCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Keys resolved"})

	return g.StatusUpdate(ctx, instance)
}

func (g handleCert) setupCert(ctx context.Context, instance *v1alpha1.Fulcio) (*utils.FulcioCertConfig, error) {
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

	if ref := instance.Spec.Certificate.CARef; ref != nil {
		key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.RootCert = key
	} else {
		rootCert, err := utils.CreateFulcioCA(ctx, g.Client, config, instance, DeploymentName)
		if err != nil {
			return nil, err
		}
		config.RootCert = rootCert
	}

	return config, nil
}
