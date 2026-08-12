package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/serviceresolver"
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
	internalUrl, err := serviceresolver.Resolve(instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("error resolving internal URL: %w", err), instance)
	}
	instance.Status.Url = internalUrl
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
