package transitions

import (
	"context"
	"fmt"
	"reflect"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewRestartOnErrorAction[T apis.ConditionsAwareObject]() action.Action[T] {
	return &restartAction[T]{}
}

type restartAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
}

func (i restartAction[T]) Name() string {
	return "restart on error"
}

func (i restartAction[T]) CanHandle(_ context.Context, instance T) bool {

	if restarts, err := i.getRestartCount(instance); err == nil {
		return restarts > 0 && meta.IsStatusConditionTrue(instance.GetConditions(), constants.Ready)
	}

	return false
}

func (i restartAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	if err := i.setRestartCount(instance, 0); err != nil {
		return i.Error(err)
	}
	return i.StatusUpdate(ctx, instance)
}

func (i restartAction[T]) CanHandleError(_ context.Context, _ T) bool {
	return true
}

func (i restartAction[T]) HandleError(ctx context.Context, instance T) *action.Result {
	restarts, err := i.getRestartCount(instance)
	if err != nil {
		return i.Error(err)
	}
	restarts++
	err = i.setRestartCount(instance, restarts)
	if err != nil {
		return i.Error(err)
	}
	if restarts < constants.AllowedRecoveryAttempts {
		instance.SetCondition(metav1.Condition{Type: constants.Ready,
			Status: metav1.ConditionFalse, Reason: constants.Pending})
	} else {
		return i.FailWithStatusUpdate(ctx, fmt.Errorf("recovery threshold reached"), instance)
	}

	return i.StatusUpdate(ctx, instance)
}

func (i restartAction[T]) getRestartCount(instance T) (int64, error) {
	if status := reflect.ValueOf(instance).Elem().FieldByName("Status"); status.IsValid() {
		if restarts := status.FieldByName("RecoveryAttempts"); restarts.CanInt() {
			return restarts.Int(), nil
		}
	}
	return 0, fmt.Errorf("can't find RecoveryAttempts count")
}

func (i restartAction[T]) setRestartCount(instance T, count int64) error {
	if status := reflect.ValueOf(instance).Elem().FieldByName("Status"); status.IsValid() {
		if restarts := status.FieldByName("RecoveryAttempts"); restarts.CanSet() {
			restarts.SetInt(count)
			return nil
		}
	}
	return fmt.Errorf("can't set RecoveryAttempts count")
}
