package logserver

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
)

func NewCreateServiceAction() action.Action[*rhtasv1alpha1.Trillian] {
	return &createServiceAction{}
}

type createServiceAction struct {
	action.BaseAction
}

func (i createServiceAction) Name() string {
	return "create service"
}

func (i createServiceAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Trillian) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Trillian) *action.Result {

	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(actions.LogServerComponentName, actions.LogserverDeploymentName, instance.Name)

	tlsAnnotations := map[string]string{}
	if specTLS(instance).CertRef == nil {
		tlsAnnotations[annotations.TLS] = fmt.Sprintf(actions.LogServerTLSSecret, instance.Name)
	}

	ports := []v1.ServicePort{
		{
			Name:       actions.ServerPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       actions.ServerPort,
			TargetPort: intstr.FromInt32(actions.ServerPort),
		}}
	if instance.Spec.Monitoring.Enabled {
		ports = append(ports, v1.ServicePort{
			Name:       actions.MetricsPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       int32(actions.MetricsPort),
			TargetPort: intstr.FromInt32(actions.MetricsPort),
		})
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: actions.LogserverDeploymentName, Namespace: instance.Namespace},
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
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Service created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}
