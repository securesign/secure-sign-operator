package server

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"

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
		expected       bool
	}{
		{
			name:           "ingress is enabled and rekor is ready",
			conditions:     []metav1.Condition{{Type: constants.Ready, Status: metav1.ConditionTrue, Reason: constants.Ready}},
			externalAccess: true,
			expected:       true,
		},
		{
			name:           "ingress is enabled and rekor is not ready",
			conditions:     []metav1.Condition{{Type: constants.Ready, Status: metav1.ConditionFalse, Reason: constants.Pending}},
			externalAccess: true,
			expected:       false,
		},
		{
			name:           "ingress is disabled",
			conditions:     []metav1.Condition{{Type: constants.Ready, Status: metav1.ConditionTrue, Reason: constants.Ready}},
			externalAccess: false,
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
