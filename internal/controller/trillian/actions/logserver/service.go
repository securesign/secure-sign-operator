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
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
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

	// Migrate existing ClusterIP service to headless: ClusterIP is immutable,
	// so we must delete and recreate if it was previously a regular service.
	if migrated, err := i.migrateToHeadless(ctx, instance); err != nil {
		return i.Error(ctx, fmt.Errorf("could not migrate service to headless: %w", err), instance)
	} else if migrated {
		return i.Requeue()
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: actions.LogserverDeploymentName, Namespace: instance.Namespace},
		},
		kubernetes.EnsureHeadlessServiceSpec(labels, ports...),
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
			Reason:  state.Creating.String(),
			Message: "Service created",
		})
		return i.StatusUpdate(ctx, instance)
	} else {
		return i.Continue()
	}
}

// migrateToHeadless checks if the existing trillian-logserver service has a
// ClusterIP assigned (non-headless). Since ClusterIP is an immutable field,
// the service must be deleted so it can be recreated as headless on the next
// reconciliation. Headless services are required for gRPC client-side load
// balancing (round_robin) because DNS must return individual pod IPs.
func (i createServiceAction) migrateToHeadless(ctx context.Context, instance *rhtasv1alpha1.Trillian) (bool, error) {
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
