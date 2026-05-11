package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
)

func NewStatusUrlAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &statusUrlAction{}
}

type statusUrlAction struct {
	action.BaseAction
}

func (i statusUrlAction) Name() string {
	return "status-url"
}

func (i statusUrlAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i statusUrlAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	protocol := "http"
	if instance.Status.TLS.CertRef != nil {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://%s.%s.svc", protocol, DeploymentName, instance.Namespace)

	if url == instance.Status.Url {
		return i.Continue()
	}

	instance.Status.Url = url
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
