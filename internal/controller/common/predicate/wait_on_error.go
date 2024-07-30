package predicate

import (
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func WaitOnError[T apis.ConditionsAwareObject]() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(event event.UpdateEvent) bool {
			// do not requeue failed object updates
			newObj, ok := event.ObjectNew.(T)
			if !ok {
				return false
			}
			old, ok := event.ObjectOld.(T)
			if !ok {
				return false
			}
			if newC, oldC := meta.FindStatusCondition(newObj.GetConditions(), constants.Ready), meta.FindStatusCondition(old.GetConditions(), constants.Ready); oldC != nil && newC != nil {
				// object is thrown to failure
				return !(newC.Reason == constants.Error && oldC.Reason != constants.Error)
			}
			return true
		},
	}
}
