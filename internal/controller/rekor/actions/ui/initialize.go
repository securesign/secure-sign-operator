package ui

import (
	"context"

	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	commonUtils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
)

func NewInitializeAction() action.Action[rhtasv1alpha1.Rekor] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Initialize &&
		!meta.IsStatusConditionTrue(instance.Status.Conditions, actions.UICondition) &&
		utils.IsEnabled(instance.Spec.RekorSearchUI.Enabled)
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		ok  bool
		err error
	)
	labels := constants.LabelsForComponent(actions.UIComponentName, instance.Name)
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	if err != nil {
		return i.Failed(err)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Initialize,
			Message: "Waiting for deployment to be ready",
		})
		return i.StatusUpdate(ctx, instance)
	}

	protocol := "http://"
	ingress := &v12.Ingress{}
	err = i.Client.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: instance.Namespace}, ingress)
	if err != nil {
		// condition error
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}
	if len(ingress.Spec.TLS) > 0 {
		protocol = "https://"
	}

	instance.Status.RekorSearchUIUrl = protocol + ingress.Spec.Rules[0].Host
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: actions.UICondition,
		Status: metav1.ConditionTrue, Reason: constants.Ready})

	return i.StatusUpdate(ctx, instance)
}
