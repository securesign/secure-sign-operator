package predicate

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

var baseTuf = &v1alpha1.Tuf{
	ObjectMeta: metav1.ObjectMeta{
		Generation: 1,
	},
	Status: v1alpha1.TufStatus{
		Conditions: []metav1.Condition{
			{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				ObservedGeneration: 1,
			},
		},
	},
}

func TestFailurePredicate(t *testing.T) {
	tests := []struct {
		name     string
		oldObj   *v1alpha1.Tuf
		newObj   *v1alpha1.Tuf
		expected bool
	}{
		{
			name:     "No changes on failure",
			oldObj:   baseTuf.DeepCopy(),
			newObj:   baseTuf.DeepCopy(),
			expected: false,
		},
		{
			name:   "From failure to creating, no change",
			oldObj: baseTuf.DeepCopy(),
			newObj: func() *v1alpha1.Tuf {
				obj := baseTuf.DeepCopy()
				obj.Status.Conditions[0].Reason = state.Creating.String()
				return obj
			}(),
			expected: true,
		},
		{
			name: "No changes not on creating state",
			oldObj: func() *v1alpha1.Tuf {
				obj := baseTuf.DeepCopy()
				obj.Status.Conditions[0].Reason = state.Creating.String()
				return obj
			}(),
			newObj: func() *v1alpha1.Tuf {
				obj := baseTuf.DeepCopy()
				obj.Status.Conditions[0].Reason = state.Creating.String()
				return obj
			}(),
			expected: true,
		},
		{
			name:   "Generation change on failure",
			oldObj: baseTuf.DeepCopy(),
			newObj: func() *v1alpha1.Tuf {
				obj := baseTuf.DeepCopy()
				obj.Generation = 2
				return obj
			}(),
			expected: true,
		},
		{
			name:   "Annotation change on failure",
			oldObj: baseTuf.DeepCopy(),
			newObj: func() *v1alpha1.Tuf {
				obj := baseTuf.DeepCopy()
				obj.Annotations = map[string]string{"foo": "bar"}
				return obj
			}(),
			expected: true,
		},
		{
			name:   "Label change on failure",
			oldObj: baseTuf.DeepCopy(),
			newObj: func() *v1alpha1.Tuf {
				obj := baseTuf.DeepCopy()
				obj.Labels = map[string]string{"foo": "bar"}
				return obj
			}(),
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			predicate := ConfigurationChangedOnFailurePredicate[*v1alpha1.Tuf]()
			g.Expect(predicate.Update(event.UpdateEvent{
				ObjectOld: test.oldObj,
				ObjectNew: test.newObj,
			})).To(Equal(test.expected))
		})
	}
}
