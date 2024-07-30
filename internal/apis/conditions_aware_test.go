package apis

import (
	"testing"

	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type testObject struct {
	v1.Object
	conditions     []v1.Condition
	expectedResult bool
	name           string
}

func (t testObject) GetObjectKind() schema.ObjectKind {
	panic("not implemented")
}

func (t testObject) DeepCopyObject() runtime.Object {
	panic("not implemented")
}

func (t testObject) GetConditions() []v1.Condition {
	return t.conditions
}

func (t testObject) SetCondition(_ v1.Condition) {
	panic("not implemented")
}

func TestIsError(t *testing.T) {
	tests := []testObject{
		{
			conditions:     nil,
			expectedResult: false,
		},
		{
			conditions: []v1.Condition{
				{
					Type:   constants.Ready,
					Status: v1.ConditionFalse,
					Reason: constants.Error,
				},
			},
			expectedResult: true,
		},
		{
			conditions: []v1.Condition{
				{
					Type:   constants.Ready,
					Status: v1.ConditionTrue,
					Reason: constants.Ready,
				},
			},
			expectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run(t.Name(), func(t *testing.T) {
			isError := IsError(test)
			if isError != test.expectedResult {
				t.Errorf("Expected %v but got %v", test.expectedResult, isError)
			}
		})
	}
}
