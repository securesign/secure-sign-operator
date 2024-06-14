package server

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cmName = "rekor-sharding-config"
)

func NewServerConfigAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "create server config"
}

func (i serverConfig) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}
	return c.Reason == constants.Creating && instance.Status.ServerConfigRef == nil
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	var (
		err error
	)
	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	if err != nil {
		return i.ErrorWithStatusUpdate(ctx, err, instance)
	}
	newConfig := kubernetes.CreateImmutableConfigmap(fmt.Sprintf("rekor-server-config-%s", instance.Namespace), instance.Namespace, labels, map[string]string{"sharding-config.yaml": ""})
	if err = controllerutil.SetControllerReference(instance, newConfig, i.Client.Scheme()); err != nil {
		return i.Error(fmt.Errorf("could not set controller reference for ConfigMap: %w", err))
	}

	_, err = i.Ensure(ctx, newConfig)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.ErrorWithStatusUpdate(ctx, err, instance)
	}

	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Creating,
		Message: "Server config created",
	})
	return i.StatusUpdate(ctx, instance)
}

func (i serverConfig) CanHandleError(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	return !meta.IsStatusConditionTrue(instance.GetConditions(), actions.ServerCondition) && instance.Status.ServerConfigRef != nil
}

func (i serverConfig) HandleError(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	deployment := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Status.ServerConfigRef.Name,
			Namespace: instance.Namespace,
		},
	}
	if err := i.Client.Delete(ctx, deployment); err != nil {
		i.Logger.V(1).Info("Can't delete server configuration", "error", err.Error())
	}

	instance.Status.ServerConfigRef = nil

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Recovering,
		Message: "server configuration will be recreated",
	})
	return i.StatusUpdate(ctx, instance)
}
