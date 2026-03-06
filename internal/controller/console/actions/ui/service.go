package ui

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

func NewCreateServiceAction() action.Action[*rhtasv1alpha1.Console] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Console) bool {
	fmt.Println("***********-------- INSIDE UI SERVICE CanHandle")
	return instance.Spec.Enabled && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Console) *action.Result {

	fmt.Println("***********-------- INSIDE UI SERVICE Handle")
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.UIComponentName, actions.UIComponentName, instance.Name)

	tlsAnnotations := map[string]string{}
	// if specTLS(instance).CertRef == nil {
	// 	tlsAnnotations[annotations.TLS] = fmt.Sprintf(actions.UITLSSecret, instance.Name)
	// }

	ports := []v1.ServicePort{
		{
			Name:       actions.UiServerPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       actions.UiServerPort,
			TargetPort: intstr.FromInt32(actions.UiServerPort),
		}}
	if instance.Spec.Monitoring.Enabled {
		ports = append(ports, v1.ServicePort{
			Name:       actions.UiMetricsPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       int32(actions.UiMetricsPort),
			TargetPort: intstr.FromInt32(actions.UiMetricsPort),
		})
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: actions.UIDeploymentName, Namespace: instance.Namespace},
		},
		kubernetes.EnsureServiceSpec(labels, ports...),
		ensure.ControllerReference[*v1.Service](instance, i.Client),
		ensure.Labels[*v1.Service](slices.Collect(maps.Keys(labels)), labels),
		//TLS: Annotate service
		ensure.Optional(kubernetes.IsOpenShift(), ensure.Annotations[*v1.Service]([]string{annotations.TLS}, tlsAnnotations)),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Service created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
