package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	futils "github.com/securesign/operator/internal/controller/fulcio/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewConfigMapAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &configMapAction{}
}

type configMapAction struct {
	action.BaseAction
}

func (i configMapAction) Name() string {
	return "configmap"
}

func (i configMapAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && instance.Spec.Config.Proxy.Enabled
}

func (i configMapAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		updated bool
		err     error
	)

	cm, err := futils.CreateConfigMap(instance, "ca-inject")
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could create ConfigMap: %w", err), instance)
	}

	if err = controllerutil.SetControllerReference(instance, cm, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for ConfigMap: %w", err))
	}

	if updated, err = i.Ensure(ctx, cm); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create Fulcio ConfigMap: %w", err), instance)
	}

	if updated {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "ConfigMap created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
