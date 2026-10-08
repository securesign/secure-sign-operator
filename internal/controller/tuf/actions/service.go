package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func NewServiceAction() action.Action[*rhtasv1.Tuf] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, tuf *rhtasv1.Tuf) bool {
	return state.FromInstance(tuf, constants.ReadyCondition) >= state.Creating
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	ownerRef, err := kubernetes.ControllerOwnerRef(instance, i.Client.Scheme())
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not compute owner reference: %w", err), instance)
	}

	l := labels.For(tufConstants.ComponentName, tufConstants.DeploymentName, instance.Name)

	svc := corev1ac.Service(tufConstants.DeploymentName, instance.Namespace).
		WithLabels(l).
		WithOwnerReferences(ownerRef).
		WithSpec(corev1ac.ServiceSpec().
			WithSelector(l).
			WithPorts(
				corev1ac.ServicePort().
					WithName(tufConstants.PortName).
					WithProtocol(v1.ProtocolTCP).
					WithPort(instance.Spec.Port).
					WithTargetPort(intstr.FromInt32(tufConstants.Port)),
			),
		)

	changed, err := kubernetes.Apply(ctx, i.Client, svc,
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: tufConstants.DeploymentName, Namespace: instance.Namespace}},
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
