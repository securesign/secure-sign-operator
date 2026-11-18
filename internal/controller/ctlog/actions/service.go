package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
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
)

func NewServiceAction() action.Action[*rhtasv1.CTlog] {
	return &serviceAction{}
}

type serviceAction struct {
	action.BaseAction
}

func (i serviceAction) Name() string {
	return "create service"
}

func (i serviceAction) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i serviceAction) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	labels := labels.For(ComponentName, ComponentName, instance.Name)
	var serverPort int32
	if ctlogutils.TlsEnabled(instance) {
		serverPort = 443
	} else {
		serverPort = 80
	}
	service, err := traits.Apply(
		corev1ac.Service(ComponentName, instance.Namespace).
			WithSpec(corev1ac.ServiceSpec().WithSelector(labels).WithPorts(
				corev1ac.ServicePort().
					WithName(ServerPortName).
					WithProtocol(v1.ProtocolTCP).
					WithPort(serverPort).
					WithTargetPort(intstr.FromInt32(ServerTargetPort)),
			)),
		traits.ControllerReference[*corev1ac.ServiceApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.ServiceApplyConfiguration](labels),
		//TLS: Annotate service
		traits.Optional(kubernetes.IsOpenShift() && instance.Spec.TLS.CertRef == nil,
			traits.Annotations[*corev1ac.ServiceApplyConfiguration](map[string]string{
				annotations.TLS: fmt.Sprintf(TLSSecret, instance.Name),
			})),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build service: %w", err), instance)
	}

	if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
		service.Spec.WithPorts(corev1ac.ServicePort().
			WithName(MetricsPortName).
			WithProtocol(v1.ProtocolTCP).
			WithPort(MetricsPort).
			WithTargetPort(intstr.FromInt32(MetricsPort)))
	}

	changed, err := kubernetes.Apply(ctx, i.Client, service, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: ComponentName, Namespace: instance.Namespace},
	})
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
	} else {
		return i.Continue()
	}
}
