package logserver

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"

	rhtasv1 "github.com/securesign/operator/api/v1"
	v1 "k8s.io/api/core/v1"
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

func (i createServiceAction) CanHandle(ctx context.Context, instance *rhtasv1.Trillian) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1.Trillian) *action.Result {
	labels := labels.For(actions.LogServerComponentName, actions.LogserverDeploymentName, instance.Name)

	// Migrate existing ClusterIP service to headless: ClusterIP is immutable,
	// so we must delete and recreate if it was previously a regular service.
	if migrated, err := i.migrateToHeadless(ctx, instance); err != nil {
		return i.Error(ctx, fmt.Errorf("could not migrate service to headless: %w", err), instance)
	} else if migrated {
		return i.Requeue()
	}

	service, err := traits.Apply(
		corev1ac.Service(actions.LogserverDeploymentName, instance.Namespace).
			WithSpec(corev1ac.ServiceSpec().
				WithSelector(labels).
				WithPorts(
					corev1ac.ServicePort().
						WithName(actions.ServerPortName).
						WithProtocol(v1.ProtocolTCP).
						WithPort(actions.ServerPort).
						WithTargetPort(intstr.FromInt32(actions.ServerPort)),
				).
				WithClusterIP(v1.ClusterIPNone)),
		traits.ControllerReference[*corev1ac.ServiceApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.ServiceApplyConfiguration](labels),
		//TLS: Annotate service
		traits.Optional(kubernetes.IsOpenShift() && specTLS(instance).CertRef == nil,
			traits.Annotations[*corev1ac.ServiceApplyConfiguration](map[string]string{
				annotations.TLS: fmt.Sprintf(actions.LogServerTLSSecret, instance.Name),
			})),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build service: %w", err), instance)
	}

	if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
		service.Spec.WithPorts(corev1ac.ServicePort().
			WithName(actions.MetricsPortName).
			WithProtocol(v1.ProtocolTCP).
			WithPort(actions.MetricsPort).
			WithTargetPort(intstr.FromInt32(actions.MetricsPort)))
	}

	changed, err := kubernetes.Apply(ctx, i.Client, service, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: actions.LogserverDeploymentName, Namespace: instance.Namespace},
	})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create service: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               actions.ServerCondition,
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

// migrateToHeadless checks if the existing trillian-logserver service has a
// ClusterIP assigned (non-headless). Since ClusterIP is an immutable field,
// the service must be deleted so it can be recreated as headless on the next
// reconciliation. Headless services are required for gRPC client-side load
// balancing (round_robin) because DNS must return individual pod IPs.
func (i createServiceAction) migrateToHeadless(ctx context.Context, instance *rhtasv1.Trillian) (bool, error) {
	existing := &v1.Service{}
	err := i.Client.Get(ctx, types.NamespacedName{
		Name:      actions.LogserverDeploymentName,
		Namespace: instance.Namespace,
	}, existing)
	if apiErrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if existing.Spec.ClusterIP != v1.ClusterIPNone {
		i.Logger.Info("Deleting ClusterIP service to recreate as headless for gRPC load balancing",
			"service", actions.LogserverDeploymentName)
		if err := i.Client.Delete(ctx, existing); err != nil && !apiErrors.IsNotFound(err) {
			return false, fmt.Errorf("failed to delete service: %w", err)
		}
		return true, nil
	}

	return false, nil
}
