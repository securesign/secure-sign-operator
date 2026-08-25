package ui

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/state"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/console/actions"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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
			name:           "ingress is enabled and console is ready",
			conditions:     []metav1.Condition{{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()}},
			externalAccess: true,
			expected:       true,
		},
		{
			name:           "ingress is enabled but console is not ready",
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
			instance := rhtasv1.Console{
				Spec: rhtasv1.ConsoleSpec{
					UI: rhtasv1.ConsoleUI{
						Ingress: rhtasv1.Ingress{
							Enabled: ptr.To(tt.externalAccess),
						},
					},
				},
				Status: rhtasv1.ConsoleStatus{
					Conditions: tt.conditions,
				},
			}
			action := NewIngressAction()
			g.Expect(tt.expected).To(Equal(action.CanHandle(ctx, &instance)))
		})
	}
}

func TestIngress_Handle_HAProxyThrottling(t *testing.T) {
	testAction.RunHAProxyThrottlingTests(t, testAction.HAProxyThrottlingTestConfig[*rhtasv1.Console]{
		NewInstance: func() *rhtasv1.Console {
			return &rhtasv1.Console{
				ObjectMeta: metav1.ObjectMeta{Name: "console", Namespace: "default"},
				Spec: rhtasv1.ConsoleSpec{
					UI: rhtasv1.ConsoleUI{
						Ingress: rhtasv1.Ingress{Enabled: ptr.To(true), Host: "console.example.com"},
					},
				},
				Status: rhtasv1.ConsoleStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
					},
				},
			}
		},
		NewService: func() *v1.Service {
			return &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: actions.UIDeploymentName, Namespace: "default"},
				Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Name: actions.UIPortName, Port: 80}}},
			}
		},
		NewAction:      NewIngressAction,
		SetAnnotations: func(c *rhtasv1.Console, a map[string]string) { c.Spec.UI.Ingress.Annotations = a },
		IngressName:    actions.UIDeploymentName,
		Namespace:      "default",
		DefaultTCP:     "100",
		DefaultHTTP:    "50",
		DefaultRateTCP: "100",
	})
}

func TestIngress_Handle_UserMetadataSSA(t *testing.T) {
	testAction.RunIngressUserMetadataTests(t, testAction.IngressUserMetadataTestConfig[*rhtasv1.Console]{
		NewInstance: func() *rhtasv1.Console {
			return &rhtasv1.Console{
				ObjectMeta: metav1.ObjectMeta{Name: "console", Namespace: "default"},
				Spec: rhtasv1.ConsoleSpec{
					UI: rhtasv1.ConsoleUI{
						Ingress: rhtasv1.Ingress{Enabled: ptr.To(true), Host: "console.example.com"},
					},
				},
				Status: rhtasv1.ConsoleStatus{
					Conditions: []metav1.Condition{
						{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
					},
				},
			}
		},
		NewService: func() *v1.Service {
			return &v1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: actions.UIDeploymentName, Namespace: "default"},
				Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Name: actions.UIPortName, Port: 80}}},
			}
		},
		NewAction:      NewIngressAction,
		SetAnnotations: func(c *rhtasv1.Console, a map[string]string) { c.Spec.UI.Ingress.Annotations = a },
		SetLabels:      func(c *rhtasv1.Console, l map[string]string) { c.Spec.UI.Ingress.Labels = l },
		IngressName:    actions.UIDeploymentName,
		Namespace:      "default",
	})
}
