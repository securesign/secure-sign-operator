package transitions

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const reasonReady = "Ready"

func TestEnsureConditions_CanHandle(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		supplier   func(*rhtasv1.CTlog) []string
		want       bool
	}{
		{
			name: "all conditions present",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: reasonReady},
				{Type: "B", Status: metav1.ConditionTrue, Reason: reasonReady},
			},
			supplier: func(_ *rhtasv1.CTlog) []string { return []string{"A", "B"} },
			want:     false,
		},
		{
			name: "one condition missing",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: reasonReady},
			},
			supplier: func(_ *rhtasv1.CTlog) []string { return []string{"A", "B"} },
			want:     true,
		},
		{
			name:       "all conditions missing",
			conditions: nil,
			supplier:   func(_ *rhtasv1.CTlog) []string { return []string{"A", "B"} },
			want:       true,
		},
		{
			name: "empty supplier",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: reasonReady},
			},
			supplier: func(_ *rhtasv1.CTlog) []string { return nil },
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := &rhtasv1.CTlog{
				Status: rhtasv1.CTlogStatus{
					Conditions: tt.conditions,
				},
			}
			a := ensureConditions[*rhtasv1.CTlog]{componentSupplier: tt.supplier}
			if got := a.CanHandle(context.Background(), instance); got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureConditions_Handle(t *testing.T) {
	const namespace = "default"
	nn := types.NamespacedName{Name: "test-ctlog", Namespace: namespace}

	tests := []struct {
		name       string
		conditions []metav1.Condition
		supplier   func(*rhtasv1.CTlog) []string
		verify     func(gomega.Gomega, *rhtasv1.CTlog)
	}{
		{
			name:       "adds all missing conditions",
			conditions: nil,
			supplier:   func(_ *rhtasv1.CTlog) []string { return []string{"A", "B", "C"} },
			verify: func(g gomega.Gomega, obj *rhtasv1.CTlog) {
				for _, name := range []string{"A", "B", "C"} {
					c := meta.FindStatusCondition(obj.GetConditions(), name)
					g.Expect(c).ToNot(gomega.BeNil(), "condition %s should exist", name)
					g.Expect(c.Status).To(gomega.Equal(metav1.ConditionUnknown))
					g.Expect(c.Reason).To(gomega.Equal(state.Pending.String()))
				}
			},
		},
		{
			name: "adds only missing conditions, preserves existing",
			conditions: []metav1.Condition{
				{Type: "A", Status: metav1.ConditionTrue, Reason: reasonReady},
			},
			supplier: func(_ *rhtasv1.CTlog) []string { return []string{"A", "B"} },
			verify: func(g gomega.Gomega, obj *rhtasv1.CTlog) {
				a := meta.FindStatusCondition(obj.GetConditions(), "A")
				g.Expect(a).ToNot(gomega.BeNil())
				g.Expect(a.Status).To(gomega.Equal(metav1.ConditionTrue), "existing condition should not be modified")
				g.Expect(a.Reason).To(gomega.Equal(reasonReady))

				b := meta.FindStatusCondition(obj.GetConditions(), "B")
				g.Expect(b).ToNot(gomega.BeNil(), "missing condition B should be added")
				g.Expect(b.Status).To(gomega.Equal(metav1.ConditionUnknown))
				g.Expect(b.Reason).To(gomega.Equal(state.Pending.String()))
			},
		},
		{
			name: "dynamic supplier based on instance spec",
			conditions: []metav1.Condition{
				{Type: "base", Status: metav1.ConditionTrue, Reason: reasonReady},
			},
			supplier: func(c *rhtasv1.CTlog) []string {
				conditions := []string{"base"}
				if c.Spec.TreeID != nil {
					conditions = append(conditions, "extra")
				}
				return conditions
			},
			verify: func(g gomega.Gomega, obj *rhtasv1.CTlog) {
				base := meta.FindStatusCondition(obj.GetConditions(), "base")
				g.Expect(base).ToNot(gomega.BeNil())
				g.Expect(base.Status).To(gomega.Equal(metav1.ConditionTrue))

				extra := meta.FindStatusCondition(obj.GetConditions(), "extra")
				g.Expect(extra).ToNot(gomega.BeNil(), "dynamic condition should be added")
				g.Expect(extra.Status).To(gomega.Equal(metav1.ConditionUnknown))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			ctx := context.Background()

			instance := &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nn.Name,
					Namespace: nn.Namespace,
				},
				Spec: rhtasv1.CTlogSpec{
					TreeID: func() *int64 { v := int64(123); return &v }(),
				},
				Status: rhtasv1.CTlogStatus{
					Conditions: tt.conditions,
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			a := testAction.PrepareAction(c, NewEnsureConditionsAction[*rhtasv1.CTlog](tt.supplier))

			result := a.Handle(ctx, instance)
			g.Expect(result).ToNot(gomega.BeNil())

			updated := &rhtasv1.CTlog{}
			g.Expect(c.Get(ctx, nn, updated)).To(gomega.Succeed())
			tt.verify(g, updated)
		})
	}
}
