// Package action provides test helpers for constructing [action.Result] values
// to use as expected outcomes in unit test assertions.
//
// These functions construct Result structs directly, without the side effects
// of their [action.BaseAction] counterparts. For example, [Error] returns a
// Result with the error set but does NOT log, detect terminal errors, or set
// status conditions. Use these when comparing the returned Result, and verify
// side effects separately.
package action

import (
	"time"

	"github.com/securesign/operator/internal/action"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Continue returns the expected result for [action.BaseAction.Continue] (nil).
func Continue() *action.Result {
	return nil
}

// Error returns the expected result for [action.BaseAction.Error].
// Only the error value is set — no logging or condition-setting side effects.
func Error(err error) *action.Result {
	return &action.Result{
		Result: reconcile.Result{},
		Err:    err,
	}
}

// Return returns the expected result for [action.BaseAction.Return].
func Return() *action.Result {
	return &action.Result{
		Result: reconcile.Result{},
		Err:    nil,
	}
}

// Requeue returns the expected result for [action.BaseAction.Requeue] (100ms).
func Requeue() *action.Result {
	return &action.Result{
		Result: reconcile.Result{RequeueAfter: 100 * time.Millisecond},
		Err:    nil,
	}
}

// RequeueAfter returns the expected result for [action.BaseAction.RequeueAfter].
// Use for polling assertions (typically 5 * time.Second).
func RequeueAfter(delay time.Duration) *action.Result {
	return &action.Result{
		Result: reconcile.Result{RequeueAfter: delay},
		Err:    nil,
	}
}
