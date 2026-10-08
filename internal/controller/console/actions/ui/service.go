package ui

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func NewCreateServiceAction() action.Action[*rhtasv1.Console] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "ui create service"
}

func (i createServiceAction) CanHandle(_ context.Context, instance *rhtasv1.Console) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	ownerRef, err := kubernetes.ControllerOwnerRef(instance, i.Client.Scheme())
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not compute owner reference: %w", err), instance)
	}

	l := labels.For(actions.UIComponentName, actions.UIDeploymentName, instance.Name)

	svc := corev1ac.Service(actions.UIDeploymentName, instance.Namespace).
		WithLabels(l).
		WithOwnerReferences(ownerRef).
		WithSpec(corev1ac.ServiceSpec().
			WithSelector(l).
			WithPorts(
				corev1ac.ServicePort().
					WithName(actions.UIPortName).
					WithProtocol(v1.ProtocolTCP).
					WithPort(actions.UIPort).
					WithTargetPort(intstr.FromString(actions.UIPortName)),
			),
		)

	changed, err := kubernetes.Apply(ctx, i.Client, svc,
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: actions.UIDeploymentName, Namespace: instance.Namespace}},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Service created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}
