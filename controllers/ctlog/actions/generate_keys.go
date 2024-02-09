package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/ctlog/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const KeySecretNameFormat = "ctlog-%s-keys"

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
	return instance.Status.Phase == v1alpha1.PhaseCreating && instance.Spec.PrivateKeyRef == nil
}

func (g generateKeys) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)

	config, err := utils.CreatePrivateKey()
	if err != nil {
		return g.Failed(err)
	}

	secretName := fmt.Sprintf(KeySecretNameFormat, instance.Name)

	secret := k8sutils.CreateSecret(secretName, instance.Namespace,
		map[string][]byte{
			"private": config.PrivateKey,
			"public":  config.PublicKey,
		}, labels)

	if err = controllerutil.SetControllerReference(instance, secret, g.Client.Scheme()); err != nil {
		return g.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}
	if _, err = g.Ensure(ctx, secret); err != nil {
		instance.Status.Phase = v1alpha1.PhaseError
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    string(v1alpha1.PhaseReady),
			Status:  metav1.ConditionFalse,
			Reason:  "Failure",
			Message: err.Error(),
		})
		return g.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Secret: %w", err), instance)
	}

	g.Recorder.Event(instance, v1.EventTypeNormal, "PrivateKeyCreated", "Private key created")
	// secret resource is not watched, it will not invoke new reconcile action
	// let's continue with instance update

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

	g.Recorder.Event(instance, v1.EventTypeNormal, "CTLogUpdated", "CTlog private key updated")
	return g.Update(ctx, instance)
}
