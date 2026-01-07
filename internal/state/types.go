package state

import (
	"github.com/securesign/operator/internal/apis"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type State int

const (
	None State = iota
	Failure
	NotDefined
	Pending
	Creating
	Initialize
	Ready
)

var stateName = map[State]string{
	None:       "",
	Failure:    "Failure",
	NotDefined: "NotDefined",
	Pending:    "Pending",
	Creating:   "Creating",
	Initialize: "Initialize",
	Ready:      "Ready",
}

func (ss State) String() string {
	return stateName[ss]
}

func FromReason(s string) State {
	switch s {
	case "Failure":
		return Failure
	case "NotDefined":
		return NotDefined
	case "Pending":
		return Pending
	case "Creating":
		return Creating
	case "Initialize":
		return Initialize
	case "Ready":
		return Ready
	default:
		return None
	}
}

func FromCondition(condition *v1.Condition) State {
	if condition == nil {
		return None
	}
	return FromReason(condition.Reason)
}

func FromInstance(instance apis.ConditionsAwareObject, condition string) State {
	return FromCondition(meta.FindStatusCondition(instance.GetConditions(), condition))
}
