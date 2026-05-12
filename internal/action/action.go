package action

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/securesign/operator/internal/apis"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Result holds the outcome of an action's Handle method.
// It maps directly to controller-runtime's (reconcile.Result, error) tuple.
type Result struct {
	Result reconcile.Result
	Err    error
}

// Action represents a single step in a controller's reconciliation sequence.
// Actions are executed in registration order. Each action checks whether it
// can handle the current state via CanHandle, and if so, performs work in Handle.
//
// Handle returns a *[Result] to control reconciliation flow:
//   - nil ([BaseAction.Continue]): proceed to the next action
//   - non-nil: stop the chain and return the result to controller-runtime
//
// See [BaseAction] for the available flow control and status persistence methods.
type Action[T apis.ConditionsAwareObject] interface {
	InjectClient(client client.Client)
	InjectRecorder(recorder events.EventRecorder)
	InjectLogger(logger logr.Logger)

	Name() string
	CanHandle(context.Context, T) bool
	Handle(context.Context, T) *Result
}
