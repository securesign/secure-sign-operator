package predicate

import (
	"github.com/securesign/operator/internal/apis"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ConditionChangedPredicate triggers when the specified status condition changes.
func ConditionChangedPredicate[T apis.ConditionsAwareObject](conditionType string) predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj, ok1 := e.ObjectOld.(T)
			newObj, ok2 := e.ObjectNew.(T)
			if !ok1 || !ok2 {
				return true
			}
			return !equality.Semantic.DeepEqual(
				meta.FindStatusCondition(oldObj.GetConditions(), conditionType),
				meta.FindStatusCondition(newObj.GetConditions(), conditionType),
			)
		},
	}
}
