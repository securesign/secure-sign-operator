package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewServiceAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {
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
			TargetPort: intstr.FromInt32(ServerPort),
		},
	}
	if instance.Spec.Monitoring.Enabled {
		ports = append(ports, v1.ServicePort{
			Name:       MetricsPortName,
			Protocol:   corev1.ProtocolTCP,
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
		ensure.Labels[*v1.Service](slices.Collect(maps.Keys(labels)), labels),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "Service created",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
