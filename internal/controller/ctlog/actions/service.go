package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/annotations"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/controller/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewServiceAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return c.Reason == constants.Creating || c.Reason == constants.Ready
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err    error
		result controllerutil.OperationResult
	)

	labels := labels.For(ComponentName, ComponentName, instance.Name)
	tlsAnnotations := map[string]string{}
	if instance.Spec.TLS.CertRef == nil {
		tlsAnnotations[annotations.TLS] = fmt.Sprintf(TLSSecret, instance.Name)
	}
	var serverPort int32
	if utils.TlsEnabled(instance) {
		serverPort = 443
	} else {
		serverPort = 80
	}
	ports := []v1.ServicePort{
		{
			Name:       ServerPortName,
			Protocol:   v1.ProtocolTCP,
			Port:       serverPort,
			TargetPort: intstr.FromInt32(ServerTargetPort),
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
			ObjectMeta: metav1.ObjectMeta{Name: ComponentName, Namespace: instance.Namespace},
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
