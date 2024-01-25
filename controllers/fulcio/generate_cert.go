package fulcio

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/fulcio/utils"
	v1 "k8s.io/api/core/v1"
	"maps"
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
	return (instance.Status.Phase == v1alpha1.PhasePending || instance.Status.Phase == v1alpha1.PhaseNone) &&
		(instance.Spec.Certificate.PrivateKeyRef == nil || instance.Spec.Certificate.CARef == nil)
}

func (g generateCert) Handle(ctx context.Context, instance *v1alpha1.Fulcio) (*v1alpha1.Fulcio, error) {
	if instance.Status.Phase == v1alpha1.PhaseNone {
		instance.Status.Phase = v1alpha1.PhasePending
		return instance, requeueError
	}

	if instance.Spec.Certificate.PrivateKeyRef == nil && instance.Spec.Certificate.CARef != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		return instance, fmt.Errorf("missing private key for CA certificate")
	}

	config, err := g.setupCert(instance)
	if err != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		return instance, err
	}

	labels := k8sutils.FilterCommonLabels(instance.Labels)
	labels[k8sutils.ComponentLabel] = ComponentName
	labels[k8sutils.NameLabel] = fulcioDeploymentName

	// todo
	secretLabels := map[string]string{
		constants.TufLabelNamespace + "/fulcio_v1.crt.pem": "cert",
	}
	maps.Copy(secretLabels, labels)

	secretName := fmt.Sprintf(SecretNameFormat, instance.Name)

	secret := k8sutils.CreateSecret(secretName, instance.Namespace, config.ToMap(), secretLabels)
	controllerutil.SetOwnerReference(instance, secret, g.Client.Scheme())

	if err = g.Client.Create(ctx, secret); err != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		return instance, fmt.Errorf("could not create fulcio secret: %w", err)
	}

	g.Recorder.Event(instance, v1.EventTypeNormal, "CertKeyCreated", "Certificate private key created")

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

	if err = g.Client.Update(ctx, instance); err != nil {
		return instance, err
	}
	g.Recorder.Event(instance, v1.EventTypeNormal, "FulcioCertUpdated", "Fulcio certificate updated")
	return nil, nil
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

	rootCert, err := utils.CreateFulcioCA(config, instance)
	if err != nil {
		return nil, err
	}
	config.RootCert = rootCert

	return config, nil
}
