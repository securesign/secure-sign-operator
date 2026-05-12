package action

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// BaseAction provides shared functionality for action implementations.
// Embed it in your action struct and use its methods for status persistence
// and flow control.
//
// Status persistence ([PersistStatus]) and flow control ([Return], [Requeue],
// [RequeueAfter], [Continue], [Error]) are intentionally separate concerns.
// After persisting status, the developer must explicitly choose what happens
// next in the reconciliation loop.
type BaseAction struct {
	Client   client.Client
	Recorder events.EventRecorder
	Logger   logr.Logger
}

func (action *BaseAction) InjectClient(client client.Client) {
	action.Client = client
}

func (action *BaseAction) InjectRecorder(recorder events.EventRecorder) {
	action.Recorder = recorder
}

func (action *BaseAction) InjectLogger(logger logr.Logger) {
	action.Logger = logger
}

// Continue signals the reconciler to proceed to the next action in the chain.
// Returns nil, which the reconciler loop interprets as "keep going."
func (action *BaseAction) Continue() *Result {
	return nil
}

// PersistStatus writes the in-memory status of obj to the API server.
// It retries on conflict and skips the write if the status has not changed.
//
// Returns (true, nil) when the status was written, (false, nil) when
// unchanged (no API call made), or (false, error) on failure.
func (action *BaseAction) PersistStatus(ctx context.Context, obj client.Object) (bool, error) {
	changed := false
	current := obj.DeepCopyObject().(client.Object)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var (
			currentStatus, expectedStatus *reflect.Value
			e                             error
		)

		if e = action.Client.Get(ctx, client.ObjectKeyFromObject(obj), current); e != nil {
			return e
		}

		if currentStatus, e = getStatus(current); e != nil {
			return e
		}

		if expectedStatus, e = getStatus(obj); e != nil {
			return e
		}

		if reflect.DeepEqual(expectedStatus.Interface(), currentStatus.Interface()) {
			changed = false
			return nil
		}
		if !currentStatus.CanSet() {
			return errors.New("can not set status field")
		}
		currentStatus.Set(*expectedStatus)
		if e = action.Client.Status().Update(ctx, current); e != nil {
			return e
		}
		changed = true
		return nil
	})
	return changed, err
}

// Error logs the error, persists any provided conditions to the API server,
// and returns the error to controller-runtime for retry with backoff.
// For terminal errors (wrapped with [reconcile.TerminalError]), it sets the
// Ready condition to Failure and controller-runtime will not retry.
func (action *BaseAction) Error(ctx context.Context, err error, instance apis.ConditionsAwareObject, conditions ...metav1.Condition) *Result {
	action.Logger.Error(err, "error during action execution")
	isTerminal := errors.Is(err, reconcile.TerminalError(err))

	if isTerminal || len(conditions) != 0 {
		updateErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if getErr := action.Client.Get(ctx, client.ObjectKeyFromObject(instance), instance); getErr != nil {
				return getErr
			}
			for _, condition := range conditions {
				instance.SetCondition(condition)
			}
			if isTerminal {
				instance.SetCondition(metav1.Condition{
					Type:               constants.ReadyCondition,
					Status:             metav1.ConditionFalse,
					Reason:             state.Failure.String(),
					Message:            err.Error(),
					ObservedGeneration: instance.GetGeneration(),
				})
			}
			return action.Client.Status().Update(ctx, instance)
		})
		if updateErr != nil {
			err = errors.Join(err, updateErr)
		}
	}

	return &Result{
		Err: err,
	}
}

// Return stops the action chain. The next reconciliation happens when
// a watch event fires (e.g., status update or owned resource change).
func (action *BaseAction) Return() *Result {
	return &Result{
		Result: reconcile.Result{},
		Err:    nil,
	}
}

// Requeue stops the action chain and re-reconciles after 100 milliseconds.
func (action *BaseAction) Requeue() *Result {
	return action.RequeueAfter(100 * time.Millisecond)
}

// RequeueAfter signals the reconciler to stop the action chain and re-reconcile
// after the specified delay.
func (action *BaseAction) RequeueAfter(delay time.Duration) *Result {
	return &Result{
		Result: reconcile.Result{RequeueAfter: delay},
	}
}

func getStatus(obj client.Object) (*reflect.Value, error) {
	stat := reflect.ValueOf(obj).Elem().FieldByName("Status")
	if stat == reflect.ValueOf(nil) {
		return nil, errors.New("status field not found")
	}
	if !stat.IsValid() {
		return nil, errors.New("status field is not valid")
	}
	return &stat, nil
}
