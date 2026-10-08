package db

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func NewCreateServiceAction() action.Action[*rhtasv1.Trillian] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(_ context.Context, instance *rhtasv1.Trillian) bool {
	return enabled(instance) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1.Trillian) *action.Result {
	ownerRef, err := kubernetes.ControllerOwnerRef(instance, i.Client.Scheme())
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not compute owner reference: %w", err), instance)
	}

	l := labels.For(actions.DbComponentName, actions.DbDeploymentName, instance.Name)

	tlsAnnotations := map[string]string{}
	if kubernetes.IsOpenShift() && specTLS(instance).CertRef == nil {
		tlsAnnotations[annotations.TLS] = fmt.Sprintf(actions.DatabaseTLSSecret, instance.Name)
	}

	svc := corev1ac.Service(host, instance.Namespace).
		WithLabels(l).
		WithAnnotations(tlsAnnotations).
		WithOwnerReferences(ownerRef).
		WithSpec(corev1ac.ServiceSpec().
			WithSelector(l).
			WithPorts(
				corev1ac.ServicePort().
					WithName(host).
					WithProtocol(v1.ProtocolTCP).
					WithPort(port).
					WithTargetPort(intstr.FromInt32(port)),
			),
		)

	changed, err := kubernetes.Apply(ctx, i.Client, svc,
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: host, Namespace: instance.Namespace}},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.DbCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Service created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}
