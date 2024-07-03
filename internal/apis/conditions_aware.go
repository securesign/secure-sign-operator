package apis

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConditionsAwareObject represents a CRD type that has been enabled with metav1.Conditions, it can then benefit of a series of utility methods.
type ConditionsAwareObject interface {
	client.Object
	GetConditions() []metav1.Condition
	SetCondition(newCondition metav1.Condition)
}
