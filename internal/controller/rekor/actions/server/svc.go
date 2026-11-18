package server

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"

	rhtasv1 "github.com/securesign/operator/api/v1"
)

func NewCreateServiceAction() action.Action[*rhtasv1.Rekor] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	labels := labels.For(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	service, err := traits.Apply(
		corev1ac.Service(actions.ServerDeploymentName, instance.Namespace).
			WithSpec(corev1ac.ServiceSpec().WithSelector(labels).WithPorts(
				corev1ac.ServicePort().
					WithName(actions.ServerDeploymentPortName).
					WithProtocol(v1.ProtocolTCP).
					WithPort(actions.ServerDeploymentPort).
					WithTargetPort(intstr.FromInt32(actions.ServerTargetDeploymentPort)),
			)),
		traits.ControllerReference[*corev1ac.ServiceApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.ServiceApplyConfiguration](labels),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build service: %w", err), instance)
	}

	if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
		service.Spec.WithPorts(corev1ac.ServicePort().
			WithName(actions.MetricsPortName).
			WithProtocol(v1.ProtocolTCP).
			WithPort(actions.MetricsPort).
			WithTargetPort(intstr.FromInt32(actions.MetricsPort)))
	}

	changed, err := kubernetes.Apply(ctx, i.Client, service, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: actions.ServerDeploymentName, Namespace: instance.Namespace},
	})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Service created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}
