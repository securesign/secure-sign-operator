package actions

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/state"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIngress_CanHandle(t *testing.T) {
	ctx := context.TODO()
	tests := []struct {
		name           string
		conditions     []metav1.Condition
		externalAccess bool
		expected       bool
	}{
		{
			name:           "ingress is enabled and tuf is ready",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: true,
			expected:       true,
		},
		{
			name:           "ingress is enabled and tuf is not ready",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()}},
			externalAccess: true,
			expected:       false,
		},
		{
			name:           "ingress is disabled",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: false,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := rhtasv1alpha1.Tuf{
				Spec: rhtasv1alpha1.TufSpec{
					ExternalAccess: rhtasv1alpha1.ExternalAccess{
						Enabled: tt.externalAccess,
					},
				},
				Status: rhtasv1alpha1.TufStatus{
					Conditions: tt.conditions,
				},
			}
			action := NewIngressAction()
			g.Expect(tt.expected).To(Equal(action.CanHandle(ctx, &instance)))
		})
	}
}
