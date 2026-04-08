package predicate

import (
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ConfigurationChangedOnFailurePredicate is a predicate that returns true
//   - ready condition's reason is failure and generation, annotation or label is changed
//   - ready condition is not in Failure state
//
// This is useful to stop the reconciliation loop when the deployment can't be fixed automatically and requires manual intervention.
func ConfigurationChangedOnFailurePredicate[T apis.ConditionsAwareObject]() predicate.Predicate {
	return predicate.Or(
		predicate.NewPredicateFuncs(
			func(obj client.Object) bool {
				conditionsAwareObject, ok := obj.(T)
				if !ok {
					return true
				}
				c := meta.FindStatusCondition(conditionsAwareObject.GetConditions(), constants.ReadyCondition)
				return c == nil || c.Reason != state.Failure.String()
			},
		), predicate.GenerationChangedPredicate{}, predicate.AnnotationChangedPredicate{}, predicate.LabelChangedPredicate{})
}
