package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewServiceAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(ComponentName, DeploymentName, instance.Name)

	ports := []v1.ServicePort{
		{
			Name:       ServerPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       ServerPort,
			TargetPort: intstr.FromInt32(TargetServerPort),
		}, {
			Name:       GRPCPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       GRPCPort,
			TargetPort: intstr.FromInt32(GRPCPort),
		},
	}
	if instance.Spec.Monitoring.Enabled {
		ports = append(ports, v1.ServicePort{
			Name:       MetricsPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       MetricsPort,
			TargetPort: intstr.FromInt32(MetricsPort),
		})
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: DeploymentName, Namespace: instance.Namespace},
		},
		kubernetes.EnsureServiceSpec(labels, ports...),
		ensure.ControllerReference[*v1.Service](instance, i.Client),
		ensure.Labels[*v1.Service](maps.Keys(labels), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Service created"})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
