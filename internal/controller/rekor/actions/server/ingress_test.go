package server

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/state"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
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
			name:           "ingress is enabled and rekor is ready",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: true,
			expected:       true,
		},
		{
			name:           "ingress is enabled and rekor is not ready",
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
			instance := rhtasv1.Rekor{
				Spec: rhtasv1.RekorSpec{
					Ingress: rhtasv1.Ingress{
						Enabled: ptr.To(tt.externalAccess),
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

func TestIngress_Handle_HAProxyThrottling(t *testing.T) {
	testAction.RunHAProxyThrottlingTests(t, testAction.HAProxyThrottlingTestConfig[*rhtasv1.Rekor]{
		NewInstance: func() *rhtasv1.Rekor {
			return &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: rhtasv1.RekorSpec{
					Ingress: rhtasv1.Ingress{Enabled: ptr.To(true), Host: "rekor.example.com"},
				},
				Status: rhtasv1.RekorStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
					},
				},
			}
		},
		NewService: func() *v1.Service {
			return &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: actions.ServerDeploymentName, Namespace: "default"},
				Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Name: actions.ServerDeploymentPortName, Port: 80}}},
			}
		},
		NewAction:      NewIngressAction,
		SetAnnotations: func(r *rhtasv1.Rekor, a map[string]string) { r.Spec.Ingress.Annotations = a },
		IngressName:    actions.ServerDeploymentName,
		Namespace:      "default",
		DefaultTCP:     "200",
		DefaultHTTP:    "200",
		DefaultRateTCP: "200",
	})
}

func TestIngress_Handle_UserMetadataSSA(t *testing.T) {
	testAction.RunIngressUserMetadataTests(t, testAction.IngressUserMetadataTestConfig[*rhtasv1.Rekor]{
		NewInstance: func() *rhtasv1.Rekor {
			return &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: rhtasv1.RekorSpec{
					Ingress: rhtasv1.Ingress{Enabled: ptr.To(true), Host: "rekor.example.com"},
				},
				Status: rhtasv1.RekorStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
					},
				},
			}
		},
		NewService: func() *v1.Service {
			return &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: actions.ServerDeploymentName, Namespace: "default"},
				Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Name: actions.ServerDeploymentPortName, Port: 80}}},
			}
		},
		NewAction:      NewIngressAction,
		SetAnnotations: func(r *rhtasv1.Rekor, a map[string]string) { r.Spec.Ingress.Annotations = a },
		SetLabels:      func(r *rhtasv1.Rekor, l map[string]string) { r.Spec.Ingress.Labels = l },
		IngressName:    actions.ServerDeploymentName,
		Namespace:      "default",
	})
}
