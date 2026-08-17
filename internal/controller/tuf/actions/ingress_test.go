package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/state"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
			t.Parallel()
			g := NewWithT(t)
			instance := rhtasv1.Tuf{
				Spec: rhtasv1.TufSpec{
					Ingress: rhtasv1.Ingress{
						Enabled: ptr.To(tt.externalAccess),
					},
				},
				Status: rhtasv1.TufStatus{
					Conditions: tt.conditions,
				},
			}
			action := NewIngressAction()
			g.Expect(tt.expected).To(Equal(action.CanHandle(ctx, &instance)))
		})
	}
}

// TUF does not receive HAProxy throttling defaults (see docs/ingress-throttling.md,
// "Supported components"), so this only verifies that user-supplied annotations are
// applied and that no throttling annotations leak in, even on OpenShift.
func TestIngress_Handle_Annotations(t *testing.T) {
	ctx := t.Context()

	newInstance := func(annotations map[string]string) *rhtasv1.Tuf {
		return &rhtasv1.Tuf{
			ObjectMeta: metav1.ObjectMeta{Name: "tuf", Namespace: "default"},
			Spec: rhtasv1.TufSpec{
				Ingress: rhtasv1.Ingress{Enabled: ptr.To(true), Host: "tuf.example.com", Annotations: annotations},
			},
			Status: rhtasv1.TufStatus{
				Conditions: []metav1.Condition{
					{Type: constants.ReadyCondition, Status: metav1.ConditionTrue, Reason: state.Ready.String()},
				},
			},
		}
	}
	newService := func() *v1.Service {
		return &v1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: tufConstants.DeploymentName, Namespace: "default"},
			Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Name: tufConstants.PortName, Port: 80}}},
		}
	}

	t.Run("openshift applies user annotations without HAProxy throttling", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = true

		c := testAction.FakeClientBuilder().WithObjects(newService()).Build()
		a := testAction.PrepareAction(c, NewIngressAction())

		a.Handle(ctx, newInstance(map[string]string{"custom/annotation": "value"}))

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: "default"}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/annotation", "value"))
		g.Expect(ingress.Annotations).ToNot(HaveKey("haproxy.router.openshift.io/rate-limit-connections"))
		g.Expect(ingress.Annotations).ToNot(HaveKey("haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp"))
	})

	t.Run("non-openshift applies user annotations", func(t *testing.T) {
		g := NewWithT(t)
		origOpenshift := config.Openshift
		t.Cleanup(func() { config.Openshift = origOpenshift })
		config.Openshift = false

		c := testAction.FakeClientBuilder().WithObjects(newService()).Build()
		a := testAction.PrepareAction(c, NewIngressAction())

		a.Handle(ctx, newInstance(map[string]string{"custom/annotation": "value"}))

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: "default"}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/annotation", "value"))
	})

	t.Run("annotation removed from the CR is deleted from the ingress on the next reconcile", func(t *testing.T) {
		g := NewWithT(t)
		c := testAction.FakeClientBuilder().WithObjects(newService()).Build()
		a := testAction.PrepareAction(c, NewIngressAction())

		instance := newInstance(map[string]string{"custom/one": "a", "custom/two": "b"})
		a.Handle(ctx, instance)

		ingress := &networkingv1.Ingress{}
		g.Expect(c.Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: "default"}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/two", "b"))

		instance.Spec.Ingress.Annotations = map[string]string{"custom/one": "a"}
		a.Handle(ctx, instance)

		g.Expect(c.Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: "default"}, ingress)).To(Succeed())
		g.Expect(ingress.Annotations).To(HaveKeyWithValue("custom/one", "a"))
		g.Expect(ingress.Annotations).ToNot(HaveKey("custom/two"))
	})
}
