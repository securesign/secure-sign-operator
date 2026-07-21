package actions

import (
	"context"
	"fmt"
	"net/url"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"

	rhtasv1 "github.com/securesign/operator/api/v1"
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
	protocol := "http"
	if instance.Status.TLS.CertRef != nil {
		protocol = "https"
	}
	u := url.URL{
		Scheme: protocol,
		Host:   fmt.Sprintf("%s.%s.svc", DeploymentName, instance.Namespace),
		Path:   instance.Spec.Prefix,
	}

	if u.String() == instance.Status.Url {
		return i.Continue()
	}

	instance.Status.Url = u.String()
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
