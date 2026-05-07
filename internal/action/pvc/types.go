package pvc

import (
	"github.com/securesign/operator/api/common"
	"github.com/securesign/operator/internal/apis"
)

const (
	ConditionType    = "PVC"
	ReasonCreated    = "Created"
	ReasonUpdated    = "Updated"
	ReasonSpecified  = "Specified"
	ReasonDiscovered = "Discovered"
)

func Wrapper[T apis.ConditionsAwareObject](
	getPVCSpec func(T) common.Pvc,
	getStatusPVCName func(T) string,
	setStatusPVCName func(T, string),
	isEnabledPVC func(T) bool,
) func(T) *wrapper[T] {
	return func(obj T) *wrapper[T] {
		return &wrapper[T]{
			object:         obj,
			callPVCSpec:    getPVCSpec,
			callGetPVCName: getStatusPVCName,
			callSetPVCName: setStatusPVCName,
			callEnabledPVC: isEnabledPVC,
		}
	}
}

type wrapper[T apis.ConditionsAwareObject] struct {
	object T

	callPVCSpec    func(T) common.Pvc
	callGetPVCName func(T) string
	callSetPVCName func(T, string)
	callEnabledPVC func(T) bool
}

func (c *wrapper[T]) GetPVCSpec() common.Pvc {
	return c.callPVCSpec(c.object)
}

func (c *wrapper[T]) GetStatusPVCName() string {
	return c.callGetPVCName(c.object)
}

func (c *wrapper[T]) SetStatusPVCName(pvcName string) {
	c.callSetPVCName(c.object, pvcName)
}

func (c *wrapper[T]) EnabledPVC() bool {
	return c.callEnabledPVC(c.object)
}
