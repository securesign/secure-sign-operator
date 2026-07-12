package ui

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	v2 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	return utils.IsEnabled(instance.Spec.UI.ExternalAccess.Enabled) && state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i ingressAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	var (
		result controllerutil.OperationResult
		err    error
	)
	ok := types.NamespacedName{Name: actions.UIDeploymentName, Namespace: instance.Namespace}
	l := labels.For(actions.UIComponentName, actions.UIDeploymentName, instance.Name)

	svc := &v1.Service{}
	if err := i.Client.Get(ctx, ok, svc); err != nil {
		return i.Error(ctx, fmt.Errorf("could not find service for ingress: %w", err), instance)
	}

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		&v2.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: svc.Name, Namespace: svc.Namespace},
		},
		kubernetes.EnsureIngressSpec(ctx, i.Client, *svc, instance.Spec.UI.ExternalAccess, actions.UIPortName),
		ensure.Optional(kubernetes.IsOpenShift(), kubernetes.EnsureIngressTLS()),
		ensure.Labels[*v2.Ingress](slices.Collect(maps.Keys(instance.Spec.UI.ExternalAccess.RouteSelectorLabels)), instance.Spec.UI.ExternalAccess.RouteSelectorLabels),
		ensure.Labels[*v2.Ingress](slices.Collect(maps.Keys(l)), l),
		ensure.ControllerReference[*v2.Ingress](instance, i.Client),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create ingress object: %w", err), instance)
	}

	if result != controllerutil.OperationResultNone {
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
