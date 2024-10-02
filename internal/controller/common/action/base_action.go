package action

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"strconv"
	"time"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/annotations"
	"k8s.io/apimachinery/pkg/api/equality"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/go-logr/logr"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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

func (action *BaseAction) StatusUpdate(ctx context.Context, expected client2.Object) *Result {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var (
			current client2.Object
			ok      bool
		)
		if current, ok = expected.DeepCopyObject().(client2.Object); !ok {
			return errors.New("can't create DeepCopy object")
		}

		if getErr := action.Client.Get(ctx, client2.ObjectKeyFromObject(expected), current); getErr != nil {
			return getErr
		}

		currentStatus := reflect.ValueOf(current).Elem().FieldByName("Status")
		expectedStatus := reflect.ValueOf(expected).Elem().FieldByName("Status")
		if currentStatus == reflect.ValueOf(nil) || expectedStatus == reflect.ValueOf(nil) {
			// object without Status
			return errors.New("can't find Status field on object")
		}
		if !expectedStatus.IsValid() || !currentStatus.IsValid() {
			return errors.New("status is not valid")
		}
		if !currentStatus.CanSet() {
			return errors.New("can't set expected Status to current object")
		}

		currentStatus.Set(expectedStatus)

		return action.Client.Status().Update(ctx, current)

	})
	return &Result{Result: reconcile.Result{Requeue: false}, Err: err}
}

func (action *BaseAction) Failed(err error) *Result {
	action.Logger.Error(err, "error during action execution")
	// If the returned error is non-nil, the Result is ignored and the request will be
	// requeued using exponential backoff.
	return &Result{
		Err: err,
	}
}

func (action *BaseAction) FailedWithStatusUpdate(ctx context.Context, err error, instance apis.ConditionsAwareObject) *Result {
	update := action.StatusUpdate(ctx, instance)
	return action.Failed(errors.Join(err, update.Err))
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

	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apiErrors.IsConflict(err) || apiErrors.IsAlreadyExists(err)
	}, func() error {
		var createUpdateError error
		result, createUpdateError = controllerutil.CreateOrUpdate(ctx, action.Client, obj, func() error {
			annoStr, find := obj.GetAnnotations()[annotations.PausedReconciliation]
			if find {
				annoBool, _ := strconv.ParseBool(annoStr)
				if annoBool {
					return nil
				}
			}

			for _, opt := range opts {
				optError := opt(obj, expected)
				if optError != nil {
					return optError
				}
			}

			return nil
		})
		return createUpdateError
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

func EnsureNTPConfig() EnsureOption {
	return func(current client.Object, expected client.Object) error {
		currentTSA, ok1 := current.(*rhtasv1alpha1.TimestampAuthority)
		expectedTSA, ok2 := expected.(*rhtasv1alpha1.TimestampAuthority)
		if !ok1 || !ok2 {
			return fmt.Errorf("EnsureNTPConfig: objects are not of type *rhtasv1alpha1.TimestampAuthority")
		}
		currentTSA.Spec.NTPMonitoring = expectedTSA.Spec.NTPMonitoring
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
