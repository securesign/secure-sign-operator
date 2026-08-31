package actions

import (
	"context"
	"fmt"
	"net/url"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	v2 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1 "github.com/securesign/operator/api/v1"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
)

func NewStatusUrlAction() action.Action[*rhtasv1.CTlog] {
	return &statusUrlAction{}
}

type statusUrlAction struct {
	action.BaseAction
}

func (i statusUrlAction) Name() string {
	return "status-url"
}

func (i statusUrlAction) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statusUrlAction) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	resolvedUrl, err := ResolveUrl(ctx, i.Client, instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("error resolving URL: %w", err), instance)
	}
	if resolvedUrl == instance.Status.Url {
		return i.Continue()
	}
	instance.Status.Url = resolvedUrl
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

// ResolveUrl returns CTlog's externally reachable URL when ingress is
// enabled, otherwise its internal cluster-DNS URL. Used for both
// Status.Url and the ctlog-monitor's own dial target, which must match
// exactly or the monitor can't find itself in the trusted root.
func ResolveUrl(ctx context.Context, cli client.Client, instance *rhtasv1.CTlog) (string, error) {
	if utils.IsEnabled(instance.Spec.Ingress.Enabled) {
		scheme := "http"
		ingress := &v2.Ingress{}
		if err := cli.Get(ctx, types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}, ingress); err != nil {
			return "", err
		}
		if len(ingress.Spec.TLS) > 0 {
			scheme = "https"
		}
		prefix := ctlogutils.ActiveLogPrefix(instance.Spec.Logs)
		return (&url.URL{Scheme: scheme, Host: ingress.Spec.Rules[0].Host, Path: prefix}).String(), nil
	}
	return serviceresolver.Resolve(instance)
}
