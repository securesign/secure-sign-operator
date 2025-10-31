package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewPvcConfigAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &pvcConfigAction{}
}

type pvcConfigAction struct {
	action.BaseAction
}

func (a pvcConfigAction) Name() string {
	return "ensure TUF PVC ConfigMap"
}

func (a pvcConfigAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Tuf) bool {
	if instance.Status.PvcName == "" {
		return false
	}

	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}

	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i pvcConfigAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tufConstants.TufPvcConfigName,
			Namespace: instance.Namespace,
		},
	}

	data := map[string]string{
		"pvcName": instance.Status.PvcName,
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		configMap,
		ensure.ControllerReference[*corev1.ConfigMap](instance, i.Client),
		kubernetes.EnsureConfigMapData(true, data),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create TUF PVC config: %w", err), instance)
	}

	i.Logger.Info("TUF PVC ConfigMap ensured", "name", configMap.Name, "pvcName", instance.Status.PvcName)

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "TUF PVC Config created",
		})
		_ = i.StatusUpdate(ctx, instance)
	}
	return i.Continue()

}
