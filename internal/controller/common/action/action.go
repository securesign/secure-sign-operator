package action

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/securesign/operator/internal/apis"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Result struct {
	Result reconcile.Result
	Err    error
}

type Action[T apis.ConditionsAwareObject] interface {
	InjectClient(client client.Client)
	InjectRecorder(recorder record.EventRecorder)
	InjectLogger(logger logr.Logger)

	// Name a user friendly name for the action
	Name() string

	// CanHandle returns true if the action can handle
	CanHandle(context.Context, T) bool

	// Handle executes the handling function
	Handle(context.Context, T) *Result

	// CanHandleError returns true if the action can handle the error
	CanHandleError(context.Context, T) bool

	// HandleError executes the error handling function for specific action
	HandleError(context.Context, T) *Result
}
