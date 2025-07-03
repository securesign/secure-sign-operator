package ui

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/securesign/operator/api/v1alpha1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIngress_CanHandle(t *testing.T) {
	ctx := context.TODO()
	g := NewWithT(t)
	tests := []struct {
		name           string
		conditions     []metav1.Condition
		externalAccess bool
		uiEnabled      bool
		expected       bool
	}{
		{
			name:           "ingress is enabled and ui is enabled and ready",
			conditions:     []metav1.Condition{{Type: constants.Ready, Status: metav1.ConditionTrue, Reason: constants.Ready}},
			externalAccess: true,
			uiEnabled:      true,
			expected:       true,
		},
		{
			name:           "ingress is enabled and ui is enabled but not ready",
			conditions:     []metav1.Condition{{Type: constants.Ready, Status: metav1.ConditionFalse, Reason: constants.Pending}},
			externalAccess: true,
			uiEnabled:      true,
			expected:       false,
		},
		{
			name:           "ingress is disabled",
			conditions:     []metav1.Condition{{Type: constants.Ready, Status: metav1.ConditionTrue, Reason: constants.Ready}},
			externalAccess: false,
			uiEnabled:      true,
			expected:       false,
		},
		{
			name:           "ingress is enabled but ui is disabled",
			conditions:     []metav1.Condition{{Type: constants.Ready, Status: metav1.ConditionTrue, Reason: constants.Ready}},
			externalAccess: true,
			uiEnabled:      false,
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := rhtasv1alpha1.Rekor{
				Spec: rhtasv1alpha1.RekorSpec{
					ExternalAccess: rhtasv1alpha1.ExternalAccess{
						Enabled: tt.externalAccess,
					},
					RekorSearchUI: v1alpha1.RekorSearchUI{
						Enabled: &tt.uiEnabled,
					},
				},
				Status: rhtasv1alpha1.RekorStatus{
					Conditions: tt.conditions,
				},
			}
			action := NewIngressAction()
			g.Expect(tt.expected).To(Equal(action.CanHandle(ctx, &instance)))
		})
	}
}
