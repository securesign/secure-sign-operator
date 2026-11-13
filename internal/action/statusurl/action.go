package statusurl

import (
	"context"
	"fmt"
	"net/url"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

type statusResolver[T apis.ConditionsAwareObject] interface {
	IsEnabled(instance T) bool
	GetIngressName() string
	GetStatusURL(instance T) string
	SetStatusURL(instance T, url string)
	GetIngressPath(instance T) string
}

type statusURLAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	resolver statusResolver[T]
}

func NewAction[T apis.ConditionsAwareObject](resolver statusResolver[T]) action.Action[T] {
	return &statusURLAction[T]{resolver: resolver}
}

func (i statusURLAction[T]) Name() string {
	return "status-url"
}

func (i statusURLAction[T]) CanHandle(_ context.Context, instance T) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statusURLAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	var u *url.URL
	if i.resolver.IsEnabled(instance) {

		ingress := &networkingv1.Ingress{}
		err := i.Client.Get(ctx, types.NamespacedName{Name: i.resolver.GetIngressName(), Namespace: instance.GetNamespace()}, ingress)
		if err != nil {
			return i.Error(ctx, err, instance)
		}
		if termination, ok := ingress.GetAnnotations()[annotations.IngressTermination]; ok && slices.Contains([]string{traits.IngressTerminationEdge, traits.IngressTerminationReencrypt, traits.IngressTerminationPassthrough}, termination) {
			u = &url.URL{Scheme: "https"}
		} else {
			u = &url.URL{Scheme: "http"}
		}
		u.Host = ingress.Spec.Rules[0].Host
		u.Path = i.resolver.GetIngressPath(instance)
	} else {
		internalURL, err := serviceresolver.Resolve(instance)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("error resolving internal URL: %w", err), instance)
		}
		u, err = url.Parse(internalURL)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("error parsing internal URL: %w", err), instance)
		}
	}

	if u.String() == i.resolver.GetStatusURL(instance) {
		return i.Continue()
	}

	i.resolver.SetStatusURL(instance, u.String())
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
