package predicate

import (
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func StopOnFailure[T apis.ConditionsAwareObject]() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(event event.UpdateEvent) bool {
			old, ok := event.ObjectOld.(T)
			if !ok {
				return false
			}

			if c := meta.FindStatusCondition(old.GetConditions(), constants.Ready); c != nil {
				return c.Reason != constants.Failure
			}

			return true
		},
	}
}
