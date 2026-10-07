package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func NewServiceAction() action.Action[*rhtasv1.TimestampAuthority] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, instance *rhtasv1.TimestampAuthority) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1.TimestampAuthority) *action.Result {
	ownerRef, err := kubernetes.ControllerOwnerRef(instance, i.Client.Scheme())
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not compute owner reference: %w", err), instance)
	}

	l := labels.For(ComponentName, DeploymentName, instance.Name)

	ports := []*corev1ac.ServicePortApplyConfiguration{
		corev1ac.ServicePort().
			WithName(ServerPortName).
			WithProtocol(v1.ProtocolTCP).
			WithPort(ServerPort).
			WithTargetPort(intstr.FromInt32(ServerPort)),
	}
	if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
		ports = append(ports, corev1ac.ServicePort().
			WithName(MetricsPortName).
			WithProtocol(v1.ProtocolTCP).
			WithPort(MetricsPort).
			WithTargetPort(intstr.FromInt32(MetricsPort)),
		)
	}

	svc := corev1ac.Service(DeploymentName, instance.Namespace).
		WithLabels(l).
		WithOwnerReferences(ownerRef).
		WithSpec(corev1ac.ServiceSpec().
			WithSelector(l).
			WithPorts(ports...),
		)

	changed, err := kubernetes.Apply(ctx, i.Client, svc,
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: DeploymentName, Namespace: instance.Namespace}},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Service created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}
