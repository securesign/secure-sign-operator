package ui

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
)

func NewStatusUrlAction() action.Action[*rhtasv1.Console] {
	return &statusUrlAction{}
}

type statusUrlAction struct {
	action.BaseAction
}

func (i statusUrlAction) Name() string {
	return "ui status-url"
}

func (i statusUrlAction) CanHandle(_ context.Context, instance *rhtasv1.Console) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statusUrlAction) Handle(ctx context.Context, instance *rhtasv1.Console) *action.Result {
	var url string
	if utils.IsEnabled(instance.Spec.UI.Ingress.Enabled) {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err := i.Client.Get(ctx, types.NamespacedName{Name: actions.UIDeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Error(ctx, err, instance)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		url = protocol + ingress.Spec.Rules[0].Host
	} else {
		var err error
		url, err = serviceresolver.Resolve(instance)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("error resolving internal URL: %w", err), instance)
		}
	}

	if url == instance.Status.UI.Url {
		return i.Continue()
	}

	instance.Status.UI.Url = url
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
