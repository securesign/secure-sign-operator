package action

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/securesign/operator/internal/controller/annotations"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

type EnsureOption func(current client.Object, expected client.Object) error

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

func (action *BaseAction) Ensure(ctx context.Context, obj client2.Object, opts ...EnsureOption) (bool, error) {
	var (
		expected client2.Object
		ok       bool
		err      error
		result   controllerutil.OperationResult
	)

	if len(opts) == 0 {
		opts = []EnsureOption{
			EnsureSpec(),
		}
	}

	if expected, ok = obj.DeepCopyObject().(client2.Object); !ok {
		return false, errors.New("can't create DeepCopy object")
	}

	err = retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		result, err = controllerutil.CreateOrUpdate(ctx, action.Client, obj, func() error {
			annoStr, find := obj.GetAnnotations()[annotations.PausedReconciliation]
			if find {
				annoBool, _ := strconv.ParseBool(annoStr)
				if annoBool {
					return nil
				}
			}

			for _, opt := range opts {
				err = opt(obj, expected)
				if err != nil {
					return err
				}
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

func EnsureSpec() EnsureOption {
	return func(current client.Object, expected client.Object) error {
		currentSpec := reflect.ValueOf(current).Elem().FieldByName("Spec")
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
	}
}

func EnsureRouteSelectorLabels(managedLabels ...string) EnsureOption {
	return func(current client.Object, expected client.Object) error {
		if current == nil || expected == nil {
			return fmt.Errorf("nil object passed")
		}

		currentSpec := reflect.ValueOf(current).Elem().FieldByName("Spec")
		expectedSpec := reflect.ValueOf(expected).Elem().FieldByName("Spec")
		if !currentSpec.IsValid() || !expectedSpec.IsValid() {
			return nil
		}

		//Current workaround for DeepEqual vs DeepDerivative, more info here https://issues.redhat.com/browse/SECURESIGN-1393
		currentRouteSelectorLabels, expectedRouteSelectorLabels := getRouteSelectorLabels(currentSpec, expectedSpec)
		if currentRouteSelectorLabels.CanSet() &&
			!equality.Semantic.DeepEqual(currentRouteSelectorLabels.Interface(), expectedRouteSelectorLabels.Interface()) {
			currentRouteSelectorLabels.Set(expectedRouteSelectorLabels)
		}

		gvk := current.GetObjectKind().GroupVersionKind()
		if gvk.Kind == "Ingress" || gvk.Kind == "Route" {
			if !reflect.DeepEqual(current.GetLabels(), expected.GetLabels()) {
				if err := EnsureLabels(managedLabels...)(current, expected); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func EnsureLabels(managedLabels ...string) EnsureOption {
	return func(current client.Object, expected client.Object) error {
		expectedLabels := expected.GetLabels()
		if expectedLabels == nil {
			expectedLabels = map[string]string{}
		}
		currentLabels := current.GetLabels()
		if currentLabels == nil {
			currentLabels = map[string]string{}
		}
		mergedLabels := make(map[string]string)
		maps.Copy(mergedLabels, currentLabels)

		maps.DeleteFunc(mergedLabels, func(k, v string) bool {
			_, existsInExpected := expectedLabels[k]
			return !existsInExpected
		})

		for _, managedLabel := range managedLabels {
			if val, exists := expectedLabels[managedLabel]; exists {
				mergedLabels[managedLabel] = val
			}
		}
		current.SetLabels(mergedLabels)
		return nil
	}
}

func EnsureAnnotations(managedAnnotations ...string) EnsureOption {
	return func(current client.Object, expected client.Object) error {
		expectedAnno := expected.GetAnnotations()
		if expectedAnno == nil {
			expectedAnno = map[string]string{}
		}
		currentAnno := current.GetAnnotations()
		if currentAnno == nil {
			currentAnno = map[string]string{}
		}
		mergedAnnotations := make(map[string]string)
		maps.Copy(mergedAnnotations, currentAnno)

		for _, managedAnno := range managedAnnotations {
			if val, exists := expectedAnno[managedAnno]; exists {
				mergedAnnotations[managedAnno] = val
			} else {
				delete(mergedAnnotations, managedAnno)
			}
		}
		current.SetAnnotations(mergedAnnotations)
		return nil
	}
}

func getRouteSelectorLabels(currentSpec, expectedSpec reflect.Value) (reflect.Value, reflect.Value) {
	var currentRouteSelectorLabels, expectedRouteSelectorLabels reflect.Value
	getRouteSelectorLabels := func(spec reflect.Value, fieldName string) reflect.Value {
		if field := spec.FieldByName(fieldName); field.IsValid() {
			if routeSelectorLabels := field.FieldByName("RouteSelectorLabels"); routeSelectorLabels.IsValid() {
				return routeSelectorLabels
			}
		}
		return reflect.Value{}
	}

	// Handle Rekor and rekor search ui
	currentRekorLabels := getRouteSelectorLabels(currentSpec, "RekorSearchUI")
	expectedRekorLabels := getRouteSelectorLabels(expectedSpec, "RekorSearchUI")
	if currentRekorLabels.IsValid() && expectedRekorLabels.IsValid() {
		if !equality.Semantic.DeepEqual(currentRekorLabels.Interface(), expectedRekorLabels.Interface()) {
			currentRouteSelectorLabels = currentRekorLabels
			expectedRouteSelectorLabels = expectedRekorLabels
		}
	}

	//Handle the rest
	if !currentRouteSelectorLabels.IsValid() && !expectedRouteSelectorLabels.IsValid() {
		currentRouteSelectorLabels = getRouteSelectorLabels(currentSpec, "ExternalAccess")
		expectedRouteSelectorLabels = getRouteSelectorLabels(expectedSpec, "ExternalAccess")
	}
	return currentRouteSelectorLabels, expectedRouteSelectorLabels
}
