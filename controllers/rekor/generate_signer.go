package rekor

import (
	"context"
	"fmt"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/utils"
	v1 "k8s.io/api/core/v1"
	"maps"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const SecretNameFormat = "rekor-%s-signer"

func NewGenerateSignerAction() action.Action[v1alpha1.Rekor] {
	return &generateSigner{}
}

type generateSigner struct {
	action.BaseAction
}

func (g generateSigner) Name() string {
	return "generate-signer"
}

func (g generateSigner) CanHandle(instance *v1alpha1.Rekor) bool {
	return (instance.Status.Phase == v1alpha1.PhasePending || instance.Status.Phase == v1alpha1.PhaseNone) &&
		(instance.Spec.Signer.KMS == "secret" || instance.Spec.Signer.KMS == "") &&
		instance.Spec.Signer.KeyRef == nil
}

func (g generateSigner) Handle(ctx context.Context, instance *v1alpha1.Rekor) (*v1alpha1.Rekor, error) {
	if instance.Status.Phase == v1alpha1.PhaseNone {
		instance.Status.Phase = v1alpha1.PhasePending
		return instance, nil
	}

	rekorServerLabels := k8sutils.FilterCommonLabels(instance.Labels)
	rekorServerLabels[k8sutils.ComponentLabel] = ComponentName
	rekorServerLabels[k8sutils.NameLabel] = RekorDeploymentName

	certConfig, err := utils.CreateRekorKey()
	if err != nil {
		return instance, err
	}

	secretLabels := map[string]string{
		constants.TufLabelNamespace + "/rekor.pub": "public",
	}
	maps.Copy(secretLabels, rekorServerLabels)

	secretName := fmt.Sprintf(SecretNameFormat, instance.Name)

	secret := k8sutils.CreateSecret(secretName, instance.Namespace,
		map[string][]byte{
			"private": certConfig.RekorKey,
			"public":  certConfig.RekorPubKey,
		}, secretLabels)
	controllerutil.SetOwnerReference(instance, secret, g.Client.Scheme())

	if err = g.Client.Create(ctx, secret); err != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		return instance, fmt.Errorf("could not create rekor secret: %w", err)
	}

	g.Recorder.Event(instance, v1.EventTypeNormal, "SignerKeyCreated", "Signer private key created")

	instance.Spec.Signer.KeyRef = &v1alpha1.SecretKeySelector{
		Key: "private",
		LocalObjectReference: v1.LocalObjectReference{
			Name: secretName,
		},
	}

	if err = g.Client.Update(ctx, instance); err != nil {
		return instance, err
	}
	g.Recorder.Event(instance, v1.EventTypeNormal, "RekorSignerUpdated", "Rekor signer key updated")
	return nil, nil
}
