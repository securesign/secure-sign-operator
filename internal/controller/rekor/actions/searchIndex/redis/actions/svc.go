package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewCreateServiceAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && enabled(instance)
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {

	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.RedisComponentName, actions.RedisDeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: actions.RedisDeploymentName, Namespace: instance.Namespace},
		},
		kubernetes.EnsureServiceSpec(labels, v1.ServicePort{
			Name:       actions.RedisDeploymentPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       actions.RedisDeploymentPort,
			TargetPort: intstr.FromInt32(actions.RedisDeploymentPort),
		}),
		ensure.ControllerReference[*v1.Service](instance, i.Client),
		ensure.Labels[*v1.Service](slices.Collect(maps.Keys(labels)), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.RedisCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Service created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
