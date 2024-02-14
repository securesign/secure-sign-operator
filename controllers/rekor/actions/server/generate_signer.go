package server

import (
	"context"
	"fmt"
	"maps"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	"github.com/securesign/operator/controllers/rekor/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const SecretNameFormat = "rekor-%s-signer"

const RekorPubLabel = constants.LabelNamespace + "/rekor.pub"

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
	return (instance.Status.Phase == v1alpha1.PhaseCreating) &&
		(instance.Spec.Signer.KMS == "secret" || instance.Spec.Signer.KMS == "") &&
		instance.Spec.Signer.KeyRef == nil
}

func (g generateSigner) Handle(ctx context.Context, instance *v1alpha1.Rekor) *action.Result {
	var (
		err     error
		updated bool
	)

	rekorServerLabels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	certConfig, err := utils.CreateRekorKey()
	if err != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		return g.FailedWithStatusUpdate(ctx, err, instance)
	}

	secretLabels := map[string]string{
		RekorPubLabel: "public",
	}
	maps.Copy(secretLabels, rekorServerLabels)

	secretName := fmt.Sprintf(SecretNameFormat, instance.Name)

	secret := k8sutils.CreateSecret(secretName, instance.Namespace,
		map[string][]byte{
			"private": certConfig.RekorKey,
			"public":  certConfig.RekorPubKey,
		}, secretLabels)
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
	g.Recorder.Event(instance, v1.EventTypeNormal, "SignerKeyCreated", "Signer private key created")

	if updated {
		instance.Spec.Signer.KeyRef = &v1alpha1.SecretKeySelector{
			Key: "private",
			LocalObjectReference: v1.LocalObjectReference{
				Name: secretName,
			},
		}
		g.Recorder.Event(instance, v1.EventTypeNormal, "RekorSignerUpdated", "Rekor signer key updated")
		return g.Update(ctx, instance)
	}
	return g.Continue()
}
