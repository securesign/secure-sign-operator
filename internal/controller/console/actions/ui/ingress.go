package ui

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	networkingv1ac "k8s.io/client-go/applyconfigurations/networking/v1"
)

func NewIngressAction() action.Action[*rhtasv1.Console] {
	return &ingressAction{}
}

type ingressAction struct {
	action.BaseAction
}

func (i ingressAction) Name() string {
	return "ui ingress"
}

func (i ingressAction) CanHandle(_ context.Context, instance *rhtasv1.Console) bool {
	return utils.IsEnabled(instance.Spec.UI.Ingress.Enabled) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i ingressAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	ok := types.NamespacedName{Name: actions.UIDeploymentName, Namespace: instance.Namespace}
	l := labels.For(actions.UIComponentName, actions.UIDeploymentName, instance.Name)

	svc := &v1.Service{}
	if err := i.Client.Get(ctx, ok, svc); err != nil {
		return i.Error(ctx, fmt.Errorf("could not find service for ingress: %w", err), instance)
	}

	ingress, err := traits.Apply(
		networkingv1ac.Ingress(svc.Name, svc.Namespace),
		traits.Labels[*networkingv1ac.IngressApplyConfiguration](instance.Spec.UI.Ingress.Labels),
		traits.Labels[*networkingv1ac.IngressApplyConfiguration](l),
		traits.ControllerReference[*networkingv1ac.IngressApplyConfiguration](instance, i.Client),
		traits.Optional(kubernetes.IsOpenShift(), traits.IngressTLSWithTermination(traits.IngressTerminationEdge)),
		traits.WithRule(
			traits.IngressHost(ctx, i.Client, instance.Spec.UI.Ingress, *svc),
			traits.IngressPath(*svc, actions.UIPortName, "/"),
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
			Type:    actions.UICondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Ingress created",
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}
	return i.Continue()
}
