package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	v12 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"

	rhtasv1 "github.com/securesign/operator/api/v1"
)

func NewStatusUrlAction() action.Action[*rhtasv1.Fulcio] {
	return &statusUrlAction{}
}

type statusUrlAction struct {
	action.BaseAction
}

func (i statusUrlAction) Name() string {
	return "status-url"
}

func (i statusUrlAction) CanHandle(_ context.Context, instance *rhtasv1.Fulcio) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statusUrlAction) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	var url string
	if instance.Spec.ExternalAccess.Enabled {
		protocol := "http://"
		ingress := &v12.Ingress{}
		err := i.Client.Get(ctx, types.NamespacedName{Name: DeploymentName, Namespace: instance.Namespace}, ingress)
		if err != nil {
			return i.Error(ctx, err, instance)
		}
		if len(ingress.Spec.TLS) > 0 {
			protocol = "https://"
		}
		url = protocol + ingress.Spec.Rules[0].Host
	} else {
		url = fmt.Sprintf("http://%s.%s.svc", DeploymentName, instance.Namespace)
	}

	if url == instance.Status.Url {
		return i.Continue()
	}

	instance.Status.Url = url
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
