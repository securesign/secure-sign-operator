package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewServiceAction() action.Action[*rhtasv1alpha1.Tuf] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, tuf *rhtasv1alpha1.Tuf) bool {
	return state.FromInstance(tuf, constants.ReadyCondition) >= state.Creating
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(tufConstants.ComponentName, tufConstants.DeploymentName, instance.Name)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: tufConstants.DeploymentName, Namespace: instance.Namespace},
		},
		kubernetes.EnsureServiceSpec(labels, v1.ServicePort{
			Name:       tufConstants.PortName,
			Protocol:   v1.ProtocolTCP,
			Port:       instance.Spec.Port,
			TargetPort: intstr.FromInt32(tufConstants.Port),
		}),
		ensure.ControllerReference[*v1.Service](instance, i.Client),
		ensure.Labels[*v1.Service](slices.Collect(maps.Keys(labels)), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.ReadyCondition,
			Status: metav1.ConditionFalse, Reason: state.Creating.String(), Message: "Service created",
			ObservedGeneration: instance.Generation})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}

}
