package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/state"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
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
		expected       bool
	}{
		{
			name:           "ingress is enabled and tsa is ready",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: true,
			expected:       true,
		},
		{
			name:           "ingress is enabled and tsa is not ready",
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
			t.Parallel()
			g := NewWithT(t)
			instance := rhtasv1.TimestampAuthority{
				Spec: rhtasv1.TimestampAuthoritySpec{
					Ingress: rhtasv1.Ingress{
						Enabled: ptr.To(tt.externalAccess),
					},
				},
				Status: rhtasv1.TimestampAuthorityStatus{
					Conditions: tt.conditions,
				},
			}
			action := NewIngressAction()
			g.Expect(tt.expected).To(Equal(action.CanHandle(ctx, &instance)))
		})
	}
}

func TestIngress_Handle_HAProxyThrottling(t *testing.T) {
	testAction.RunHAProxyThrottlingTests(t, testAction.HAProxyThrottlingTestConfig[*rhtasv1.TimestampAuthority]{
		NewInstance: func() *rhtasv1.TimestampAuthority {
			return &rhtasv1.TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "default"},
				Spec: rhtasv1.TimestampAuthoritySpec{
					Ingress: rhtasv1.Ingress{Enabled: ptr.To(true), Host: "tsa.example.com"},
				},
				Status: rhtasv1.TimestampAuthorityStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
					},
				},
			}
		},
		NewService: func() *v1.Service {
			return &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: DeploymentName, Namespace: "default"},
				Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Name: DeploymentName, Port: 80}}},
			}
		},
		NewAction:      NewIngressAction,
		SetAnnotations: func(tsa *rhtasv1.TimestampAuthority, a map[string]string) { tsa.Spec.Ingress.Annotations = a },
		IngressName:    DeploymentName,
		Namespace:      "default",
		DefaultTCP:     "100",
		DefaultHTTP:    "100",
		DefaultRateTCP: "100",
	})
}

func TestIngress_Handle_UserMetadataSSA(t *testing.T) {
	testAction.RunIngressUserMetadataTests(t, testAction.IngressUserMetadataTestConfig[*rhtasv1.TimestampAuthority]{
		NewInstance: func() *rhtasv1.TimestampAuthority {
			return &rhtasv1.TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "default"},
				Spec: rhtasv1.TimestampAuthoritySpec{
					Ingress: rhtasv1.Ingress{Enabled: ptr.To(true), Host: "tsa.example.com"},
				},
				Status: rhtasv1.TimestampAuthorityStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
					},
				},
			}
		},
		NewService: func() *v1.Service {
			return &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: DeploymentName, Namespace: "default"},
				Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Name: DeploymentName, Port: 80}}},
			}
		},
		NewAction:      NewIngressAction,
		SetAnnotations: func(tsa *rhtasv1.TimestampAuthority, a map[string]string) { tsa.Spec.Ingress.Annotations = a },
		SetLabels:      func(tsa *rhtasv1.TimestampAuthority, l map[string]string) { tsa.Spec.Ingress.Labels = l },
		IngressName:    DeploymentName,
		Namespace:      "default",
	})
}
