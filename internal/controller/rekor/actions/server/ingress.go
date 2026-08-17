package server

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v2 "k8s.io/api/networking/v1"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// rekorThrottlingDefaults are the default HAProxy rate-limiting values applied
// when the CR does not override them. Every sign and verify operation hits
// Rekor, and TUF clients can fetch multiple files per refresh, so Rekor gets
// higher defaults than the other components.
var rekorThrottlingDefaults = kubernetes.IngressThrottlingDefaults{
	ConcurrentTCP: 200,
	RateHTTP:      200,
	RateTCP:       200,
}

func NewIngressAction() action.Action[*rhtasv1.Rekor] {
	return &ingressAction{}
}

type ingressAction struct {
	action.BaseAction
}

func (i ingressAction) Name() string {
	return "ingress"
}

func (i ingressAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return utils.IsEnabled(instance.Spec.Ingress.Enabled) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i ingressAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)
	ok := types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}
	labels := labels.For(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

	svc := &v1.Service{}
	if err := i.Client.Get(ctx, ok, svc); err != nil {
		return i.Error(ctx, fmt.Errorf("could not find service for ingress: %w", err), instance)
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v2.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: svc.Name, Namespace: svc.Namespace},
		},
		kubernetes.EnsureIngressSpec(ctx, i.Client, *svc, instance.Spec.Ingress, actions.ServerDeploymentPortName),
		ensure.Optional(kubernetes.IsOpenShift(), kubernetes.EnsureIngressTLS()),
		ensure.Optional(kubernetes.IsOpenShift(), kubernetes.EnsureIngressHAProxyThrottling(instance.Spec.Ingress.Annotations, rekorThrottlingDefaults)),
		func(ingress *v2.Ingress) error {
			if ingress.Labels == nil {
				ingress.Labels = map[string]string{}
			}
			for k := range ingress.Labels {
				if _, isComponent := labels[k]; isComponent {
					continue
				}
				if _, isCurrent := instance.Spec.Ingress.Labels[k]; !isCurrent {
					delete(ingress.Labels, k)
				}
			}
			maps.Copy(ingress.Labels, instance.Spec.Ingress.Labels)
			return nil
		},
		ensure.Labels[*v2.Ingress](slices.Collect(maps.Keys(labels)), labels),
		ensure.ControllerReference[*v2.Ingress](instance, i.Client),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create ingress object: %w", err), instance)
	}

	if err = kubernetes.ApplyIngressUserMetadata(ctx, i.Client, svc.Name, svc.Namespace, instance.Spec.Ingress.Annotations, instance.Spec.Ingress.Labels); err != nil {
		return i.Error(ctx, fmt.Errorf("could not apply user metadata to ingress: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Ingress created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	} else {
		return i.Continue()
	}
}
