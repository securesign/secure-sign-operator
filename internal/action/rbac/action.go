package rbac

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WithRule[T apis.ConditionsAwareObject](rule rbacv1.PolicyRule) func(action2 *rbacAction[T]) {
	return func(obj *rbacAction[T]) {
		obj.rules = append(obj.rules, rule)
	}
}

func WithCanHandle[T apis.ConditionsAwareObject](fn func(context.Context, T) bool) func(action2 *rbacAction[T]) {
	return func(obj *rbacAction[T]) {
		obj.canHandle = fn
	}
}

func WithImagePullSecrets[T apis.ConditionsAwareObject](fn func(T) []v1.LocalObjectReference) func(action2 *rbacAction[T]) {
	return func(obj *rbacAction[T]) {
		obj.imagePullSecrets = fn
	}
}

func NewAction[T apis.ConditionsAwareObject](componentName, rbacName string, opts ...func(action2 *rbacAction[T])) action.Action[T] {
	a := &rbacAction[T]{
		componentName: componentName,
		rbacName:      rbacName,
		rules:         make([]rbacv1.PolicyRule, 0),
		canHandle:     defaultCanHandle[T],
	}

	// Default to no image pull secrets - optional callback
	if a.imagePullSecrets == nil {
		a.imagePullSecrets = func(T) []v1.LocalObjectReference { return nil }
	}

	for _, opt := range opts {
		opt(a)
	}
	return a
}

type rbacAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	componentName    string
	rbacName         string
	rules            []rbacv1.PolicyRule
	canHandle        func(context.Context, T) bool
	imagePullSecrets func(T) []v1.LocalObjectReference
}

func (i rbacAction[T]) Name() string {
	return "RBAC"
}

func defaultCanHandle[T apis.ConditionsAwareObject](_ context.Context, instance T) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i rbacAction[T]) CanHandle(ctx context.Context, instance T) bool {
	return i.canHandle(ctx, instance)
}

func (i rbacAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	var result *action.Result

	result = i.handleServiceAccount(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleRole(ctx, instance)
	if result != nil {
		return result
	}

	result = i.handleRoleBinding(ctx, instance)
	if result != nil {
		return result
	}

	return i.Continue()
}

func (i rbacAction[T]) handleServiceAccount(ctx context.Context, instance T) *action.Result {
	l := labels.For(i.componentName, i.rbacName, instance.GetName())

	sa, err := traits.Apply(
		corev1ac.ServiceAccount(i.rbacName, instance.GetNamespace()),
		traits.ControllerReference[*corev1ac.ServiceAccountApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.ServiceAccountApplyConfiguration](l),
		traits.ImagePullSecrets(i.imagePullSecrets(instance)...),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build SA: %w", err), instance)
	}

	if _, err = kubernetes.Apply(ctx, i.Client, sa, &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.rbacName,
			Namespace: instance.GetNamespace(),
		},
	}); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create SA: %w", err), instance)
	}

	return i.Continue()
}

func (i rbacAction[T]) handleRole(ctx context.Context, instance T) *action.Result {
	var err error
	l := labels.For(i.componentName, i.rbacName, instance.GetName())

	if len(i.rules) == 0 {
		if err = i.Client.Delete(ctx, &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      i.rbacName,
				Namespace: instance.GetNamespace(),
			}}); client.IgnoreNotFound(err) != nil {
			return i.Error(ctx, fmt.Errorf("could not delete Role: %w", err), instance)
		}
		return i.Continue()
	}

	role, err := traits.Apply(
		rbacv1ac.Role(i.rbacName, instance.GetNamespace()),
		traits.ControllerReference[*rbacv1ac.RoleApplyConfiguration](instance, i.Client),
		traits.Labels[*rbacv1ac.RoleApplyConfiguration](l),
		traits.RoleRules(i.rules...),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build Role: %w", err), instance)
	}

	if _, err = kubernetes.Apply(ctx, i.Client, role, &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.rbacName,
			Namespace: instance.GetNamespace(),
		},
	}); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Role: %w", err), instance)
	}

	return i.Continue()
}

func (i rbacAction[T]) handleRoleBinding(ctx context.Context, instance T) *action.Result {
	var err error
	l := labels.For(i.componentName, i.rbacName, instance.GetName())

	if len(i.rules) == 0 {
		if err = i.Client.Delete(ctx, &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      i.rbacName,
				Namespace: instance.GetNamespace(),
			}}); client.IgnoreNotFound(err) != nil {
			return i.Error(ctx, fmt.Errorf("could not delete RoleBinding: %w", err), instance)
		}
		return i.Continue()
	}

	binding, err := traits.Apply(
		rbacv1ac.RoleBinding(i.rbacName, instance.GetNamespace()),
		traits.ControllerReference[*rbacv1ac.RoleBindingApplyConfiguration](instance, i.Client),
		traits.Labels[*rbacv1ac.RoleBindingApplyConfiguration](l),
		traits.RoleBinding(
			rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role", //nolint:goconst
				Name:     i.rbacName,
			},
			rbacv1.Subject{Kind: "ServiceAccount", Name: i.rbacName, Namespace: instance.GetNamespace()}, //nolint:goconst
		),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build RoleBinding: %w", err), instance)
	}

	if _, err = kubernetes.Apply(ctx, i.Client, binding, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      i.rbacName,
			Namespace: instance.GetNamespace(),
		},
	}); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create RoleBinding: %w", err), instance)
	}

	return i.Continue()
}
