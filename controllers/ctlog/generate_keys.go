package ctlog

import (
	"context"
	"fmt"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/ctlog/utils"
	v1 "k8s.io/api/core/v1"
	"maps"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const SecretNameFormat = "ctlog-%s-keys"

func NewGenerateKeysAction() action.Action[v1alpha1.CTlog] {
	return &generateKeys{}
}

type generateKeys struct {
	action.BaseAction
}

func (g generateKeys) Name() string {
	return "generate-keys"
}

func (g generateKeys) CanHandle(instance *v1alpha1.CTlog) bool {
	return (instance.Status.Phase == v1alpha1.PhasePending || instance.Status.Phase == v1alpha1.PhaseNone) &&
		instance.Spec.PrivateKeyRef == nil
}

func (g generateKeys) Handle(ctx context.Context, instance *v1alpha1.CTlog) (*v1alpha1.CTlog, error) {
	if instance.Status.Phase == v1alpha1.PhaseNone {
		instance.Status.Phase = v1alpha1.PhasePending
		return instance, nil
	}

	labels := k8sutils.FilterCommonLabels(instance.Labels)
	labels[k8sutils.ComponentLabel] = ComponentName
	labels[k8sutils.NameLabel] = deploymentName

	config, err := utils.CreatePrivateKey()
	if err != nil {
		return instance, err
	}

	secretLabels := map[string]string{
		constants.TufLabelNamespace + "/ctfe.pub": "public",
	}
	maps.Copy(secretLabels, labels)

	secretName := fmt.Sprintf(SecretNameFormat, instance.Name)

	secret := k8sutils.CreateSecret(secretName, instance.Namespace,
		map[string][]byte{
			"private": config.PrivateKey,
			"public":  config.PublicKey,
		}, secretLabels)
	controllerutil.SetOwnerReference(instance, secret, g.Client.Scheme())

	if err = g.Client.Create(ctx, secret); err != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		return instance, fmt.Errorf("could not create ctlog secret: %w", err)
	}

	g.Recorder.Event(instance, v1.EventTypeNormal, "PrivateKeyCreated", "Private key created")

	instance.Spec.PrivateKeyRef = &v1alpha1.SecretKeySelector{
		Key: "private",
		LocalObjectReference: v1.LocalObjectReference{
			Name: secretName,
		},
	}

	instance.Spec.PublicKeyRef = &v1alpha1.SecretKeySelector{
		Key: "public",
		LocalObjectReference: v1.LocalObjectReference{
			Name: secretName,
		},
	}

	if err = g.Client.Update(ctx, instance); err != nil {
		return instance, err
	}
	g.Recorder.Event(instance, v1.EventTypeNormal, "CTLogUpdated", "CTlog private key updated")
	return nil, nil
}
