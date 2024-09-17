package action

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/securesign/operator/internal/controller/annotations"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// OptimisticLockErrorMsg - ignore update error: https://github.com/kubernetes/kubernetes/issues/28149
const OptimisticLockErrorMsg = "the object has been modified; please apply your changes to the latest version and try again"

type BaseAction struct {
	Client   client.Client
	Recorder record.EventRecorder
	Logger   logr.Logger
}

func (action *BaseAction) InjectClient(client client.Client) {
	action.Client = client
}

func (action *BaseAction) InjectRecorder(recorder record.EventRecorder) {
	action.Recorder = recorder
}

func (action *BaseAction) InjectLogger(logger logr.Logger) {
	action.Logger = logger
}

func (action *BaseAction) Continue() *Result {
	return nil
}

func (action *BaseAction) StatusUpdate(ctx context.Context, obj client2.Object) *Result {
	if err := action.Client.Status().Update(ctx, obj); err != nil {
		if strings.Contains(err.Error(), OptimisticLockErrorMsg) {
			return &Result{Result: reconcile.Result{RequeueAfter: 1 * time.Second}, Err: nil}
		}
		return action.Failed(err)
	}
	// Requeue will be caused by update
	return &Result{Result: reconcile.Result{Requeue: false}}
}

func (action *BaseAction) Failed(err error) *Result {
	action.Logger.Error(err, "error during action execution")
	return &Result{
		Result: reconcile.Result{RequeueAfter: time.Duration(5) * time.Second},
		Err:    err,
	}
}

func (action *BaseAction) FailedWithStatusUpdate(ctx context.Context, err error, instance client2.Object) *Result {
	if e := action.Client.Status().Update(ctx, instance); e != nil {
		if strings.Contains(err.Error(), OptimisticLockErrorMsg) {
			return &Result{Result: reconcile.Result{RequeueAfter: 1 * time.Second}, Err: err}
		}
		err = errors.Join(e, err)
	}
	// Requeue will be caused by update
	return &Result{Result: reconcile.Result{Requeue: false}, Err: err}
}

func (action *BaseAction) Return() *Result {
	return &Result{
		Result: reconcile.Result{Requeue: false},
		Err:    nil,
	}
}

func (action *BaseAction) Requeue() *Result {
	return &Result{
		// always wait for a while before requeqe
		Result: reconcile.Result{RequeueAfter: 5 * time.Second},
		Err:    nil,
	}
}

func (action *BaseAction) Ensure(ctx context.Context, obj client2.Object) (bool, error) {
	var (
		expected client2.Object
		ok       bool
		err      error
		result   controllerutil.OperationResult
	)

	if expected, ok = obj.DeepCopyObject().(client2.Object); !ok {
		return false, errors.New("Can't create DeepCopy object")
	}
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err = controllerutil.CreateOrUpdate(ctx, action.Client, obj, func() error {
			annoStr, find := obj.GetAnnotations()[annotations.PausedReconciliation]
			if find {
				annoBool, _ := strconv.ParseBool(annoStr)
				if annoBool {
					return nil
				}
			}

			currentSpec := reflect.ValueOf(obj).Elem().FieldByName("Spec")
			expectedSpec := reflect.ValueOf(expected).Elem().FieldByName("Spec")
			if currentSpec == reflect.ValueOf(nil) {
				// object without spec
				// return without update
				return nil
			}
			if !expectedSpec.IsValid() || !currentSpec.IsValid() {
				return errors.New("spec is not valid")
			}
			if !currentSpec.CanSet() {
				return errors.New("can't set expected spec to current object")
			}

			// WORKAROUND: CreateOrUpdate uses DeepEqual to compare
			// DeepEqual does not honor default values
			if !equality.Semantic.DeepDerivative(expectedSpec.Interface(), currentSpec.Interface()) {
				currentSpec.Set(expectedSpec)
			}

			return nil
		})
		return err
	})

	if err != nil {
		return false, err
	}

	return result != controllerutil.OperationResultNone, nil
}
