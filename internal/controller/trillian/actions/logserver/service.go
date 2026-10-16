package logserver

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
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (i createServiceAction) CanHandle(ctx context.Context, instance *rhtasv1.Trillian) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i createServiceAction) Handle(ctx context.Context, instance *rhtasv1.Trillian) *action.Result {
	if migrated, err := i.migrateToHeadless(ctx, instance); err != nil {
		return i.Error(ctx, fmt.Errorf("could not migrate service to headless: %w", err), instance)
	} else if migrated {
		return i.Requeue()
	}

	ownerRef, err := kubernetes.ControllerOwnerRef(instance, i.Client.Scheme())
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not compute owner reference: %w", err), instance)
	}

	l := labels.For(actions.LogServerComponentName, actions.LogserverDeploymentName, instance.Name)

	tlsAnnotations := map[string]string{}
	if kubernetes.IsOpenShift() && specTLS(instance).CertRef == nil {
		tlsAnnotations[annotations.TLS] = fmt.Sprintf(actions.LogServerTLSSecret, instance.Name)
	}

	ports := []*corev1ac.ServicePortApplyConfiguration{
		corev1ac.ServicePort().
			WithName(actions.ServerPortName).
			WithProtocol(v1.ProtocolTCP).
			WithPort(actions.ServerPort).
			WithTargetPort(intstr.FromInt32(actions.ServerPort)),
	}
	if utils.IsEnabled(instance.Spec.Monitoring.Metrics.Enabled) {
		ports = append(ports, corev1ac.ServicePort().
			WithName(actions.MetricsPortName).
			WithProtocol(v1.ProtocolTCP).
			WithPort(actions.MetricsPort).
			WithTargetPort(intstr.FromInt32(actions.MetricsPort)),
		)
	}

	svc := corev1ac.Service(actions.LogserverDeploymentName, instance.Namespace).
		WithLabels(l).
		WithAnnotations(tlsAnnotations).
		WithOwnerReferences(ownerRef).
		WithSpec(corev1ac.ServiceSpec().
			WithClusterIP(v1.ClusterIPNone).
			WithSelector(l).
			WithPorts(ports...),
		)

	changed, err := kubernetes.Apply(ctx, i.Client, svc,
		&v1.Service{ObjectMeta: metav1.ObjectMeta{Name: actions.LogserverDeploymentName, Namespace: instance.Namespace}},
	)
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
	}
	return i.Continue()
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
