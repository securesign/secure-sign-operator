package action

import (
	"time"

	"github.com/securesign/operator/internal/action"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Continue() *action.Result {
	return nil
}

func StatusUpdate() *action.Result {
	return &action.Result{Result: reconcile.Result{Requeue: false}}
}

func Error(err error) *action.Result {
	return &action.Result{
		Result: reconcile.Result{},
		Err:    err,
	}
}

func Return() *action.Result {
	return &action.Result{
		Result: reconcile.Result{Requeue: false},
		Err:    nil,
	}
}

func Requeue() *action.Result {
	return &action.Result{
		Result: reconcile.Result{RequeueAfter: 5 * time.Second},
		Err:    nil,
	}
}
