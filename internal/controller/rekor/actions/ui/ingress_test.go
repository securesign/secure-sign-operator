package ui

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/state"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestIngress_CanHandle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tests := []struct {
		name           string
		conditions     []metav1.Condition
		externalAccess bool
		uiEnabled      bool
		expected       bool
	}{
		{
			name:           "ingress is enabled and ui is enabled and ready",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: true,
			uiEnabled:      true,
			expected:       true,
		},
		{
			name:           "ingress is enabled and ui is enabled but not ready",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()}},
			externalAccess: true,
			uiEnabled:      true,
			expected:       false,
		},
		{
			name:           "ingress is disabled",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: false,
			uiEnabled:      true,
			expected:       false,
		},
		{
			name:           "ingress is enabled but ui is disabled",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: true,
			uiEnabled:      false,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			instance := rhtasv1.Rekor{
				Spec: rhtasv1.RekorSpec{
					ExternalAccess: rhtasv1.ExternalAccess{
						Enabled: ptr.To(tt.externalAccess),
					},
					RekorSearchUI: rhtasv1.RekorSearchUI{
						Enabled: &tt.uiEnabled,
					},
				},
				Status: rhtasv1.RekorStatus{
					Conditions: tt.conditions,
				},
			}
			action := NewIngressAction()
			g.Expect(tt.expected).To(Equal(action.CanHandle(ctx, &instance)))
		})
	}
}
