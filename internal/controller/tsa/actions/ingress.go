package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v2 "k8s.io/api/networking/v1"

	rhtasv1 "github.com/securesign/operator/api/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	networkingv1ac "k8s.io/client-go/applyconfigurations/networking/v1"
)

func NewIngressAction() action.Action[*rhtasv1.TimestampAuthority] {
	return &ingressAction{}
}

type ingressAction struct {
	action.BaseAction
}

func (i ingressAction) Name() string {
	return "ingress"
}

func (i ingressAction) CanHandle(_ context.Context, instance *rhtasv1.TimestampAuthority) bool {
	return utils.IsEnabled(instance.Spec.Ingress.Enabled) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i ingressAction) Handle(ctx context.Context, instance *rhtasv1.TimestampAuthority) *action.Result {
	ok := types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}
	labels := labels.For(ComponentName, DeploymentName, instance.Name)

	svc := &v1.Service{}
	if err := i.Client.Get(ctx, ok, svc); err != nil {
		return i.Error(ctx, fmt.Errorf("could not find service for ingress: %w", err), instance)
	}

	ingress, err := traits.Apply(
		networkingv1ac.Ingress(svc.Name, svc.Namespace),
		traits.Labels[*networkingv1ac.IngressApplyConfiguration](instance.Spec.Ingress.Labels),
		traits.Labels[*networkingv1ac.IngressApplyConfiguration](labels),
		traits.ControllerReference[*networkingv1ac.IngressApplyConfiguration](instance, i.Client),
		traits.Optional(kubernetes.IsOpenShift(), traits.IngressTLSWithTermination(traits.IngressTerminationEdge)),
		traits.WithRule(
			traits.IngressHost(ctx, i.Client, instance.Spec.Ingress, *svc),
			traits.IngressPath(*svc, DeploymentName, "/"),
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build ingress object: %w", err), instance)
	}

	changed, err := kubernetes.Apply(ctx, i.Client, ingress,
		&v2.Ingress{ObjectMeta: metav1.ObjectMeta{Name: svc.Name, Namespace: svc.Namespace}},
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not apply ingress object: %w", err), instance)
	}

	if changed {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Ingress created",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}
