package predicate

import (
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

/*
IgnoreFailurePredicate is a predicate that ignores objects where the Ready condition is False and the Reason is Failure.
This is useful to stop the reconciliation loop when the deployment can't be fixed automatically and requires manual intervention.
*/
func IgnoreFailurePredicate[T apis.ConditionsAwareObject]() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		conditionsAwareObject, ok := obj.(T)
		if !ok {
			return true
		}

		c := meta.FindStatusCondition(conditionsAwareObject.GetConditions(), constants.ReadyCondition)

		if c != nil && c.Status == metav1.ConditionFalse && c.Reason == state.Failure.String() {
			return false
		}
		return true
	})
}
