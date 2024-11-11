package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	commonUtils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NewInitializeAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return &initializeAction{}
}

type initializeAction struct {
	action.BaseAction
}

func (i initializeAction) Name() string {
	return "initialize"
}

func (i initializeAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	if c == nil {
		return false
	}
	return c.Reason == constants.Initialize
}

func (i initializeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {
	var (
		ok  bool
		err error
	)
	labels := labels.ForComponent(ComponentName, instance.Name)
	ok, err = commonUtils.DeploymentIsRunning(ctx, i.Client, instance.Namespace, labels)
	switch {
	case errors.Is(err, commonUtils.ErrDeploymentNotReady):
		i.Logger.Error(err, "deployment is not ready")
	case err != nil:
		return i.Failed(err)
	}
	if !ok {
		i.Logger.Info("Waiting for deployment")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Initialize,
			Message:            "Waiting for deployment to be ready",
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSAServerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Initialize,
			Message:            "Waiting for deployment to be ready",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	}

	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err = i.Client.Get(ctx, types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Failed(err)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		instance.Status.Url = protocol + ingress.Spec.Rules[0].Host
	} else {
		instance.Status.Url = fmt.Sprintf("http://%s.%s.svc", DeploymentName, instance.Namespace)
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: TSAServerCondition,
		Status: metav1.ConditionTrue, Reason: constants.Ready, ObservedGeneration: instance.Generation})

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionTrue, Reason: constants.Ready, ObservedGeneration: instance.Generation})

	return i.StatusUpdate(ctx, instance)
}
