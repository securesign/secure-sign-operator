package logserver

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMigrateToHeadless(t *testing.T) {
	ctx := context.TODO()

	tests := []struct {
		name           string
		objects        []client.Object
		expectMigrated bool
		expectErr      bool
		expectDeleted  bool
	}{
		{
			name:           "no existing service - nothing to migrate",
			objects:        []client.Object{},
			expectMigrated: false,
			expectErr:      false,
			expectDeleted:  false,
		},
		{
			name: "existing ClusterIP service - should delete for migration",
			objects: []client.Object{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      actions.LogserverDeploymentName,
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						ClusterIP: "10.0.0.1",
						Selector:  map[string]string{"app": "trillian-logserver"},
						Ports: []v1.ServicePort{
							{Name: "grpc", Port: 8091},
						},
					},
				},
			},
			expectMigrated: true,
			expectErr:      false,
			expectDeleted:  true,
		},
		{
			name: "existing headless service - no migration needed",
			objects: []client.Object{
				&v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      actions.LogserverDeploymentName,
						Namespace: "default",
					},
					Spec: v1.ServiceSpec{
						ClusterIP: v1.ClusterIPNone,
						Selector:  map[string]string{"app": "trillian-logserver"},
						Ports: []v1.ServicePort{
							{Name: "grpc", Port: 8091},
						},
					},
				},
			},
			expectMigrated: false,
			expectErr:      false,
			expectDeleted:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			a := testAction.PrepareAction(c, NewCreateServiceAction())
			action := a.(*createServiceAction)

			instance := &rhtasv1alpha1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trillian",
					Namespace: "default",
				},
			}

			migrated, err := action.migrateToHeadless(ctx, instance)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(migrated).To(Equal(tt.expectMigrated))

			if tt.expectDeleted {
				svc := &v1.Service{}
				err := c.Get(ctx, client.ObjectKey{
					Name:      actions.LogserverDeploymentName,
					Namespace: "default",
				}, svc)
				g.Expect(err).To(HaveOccurred())
			}
		})
	}
}

func TestCreateServiceAction_Handle_CreatesHeadless(t *testing.T) {
	ctx := context.TODO()
	g := NewWithT(t)

	instance := &rhtasv1alpha1.Trillian{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-trillian",
			Namespace: "default",
		},
		Status: rhtasv1alpha1.TrillianStatus{
			Conditions: []metav1.Condition{
				{
					Type:   actions.ServerCondition,
					Status: metav1.ConditionFalse,
					Reason: state.Creating.String(),
				},
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewCreateServiceAction())
	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	svc := &v1.Service{}
	g.Expect(c.Get(ctx, client.ObjectKey{
		Name:      actions.LogserverDeploymentName,
		Namespace: "default",
	}, svc)).To(Succeed())
	g.Expect(svc.Spec.ClusterIP).To(Equal(v1.ClusterIPNone))
}
