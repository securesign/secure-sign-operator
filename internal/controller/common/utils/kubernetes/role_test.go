package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	testAction "github.com/securesign/operator/internal/testing/action"
	rbacv1 "k8s.io/api/rbac/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestEnsureRoleRules(t *testing.T) {
	gomega.RegisterTestingT(t)
	tests := []struct {
		name    string
		objects []client.Object
		result  controllerutil.OperationResult
	}{
		{
			"create new object",
			[]client.Object{},
			controllerutil.OperationResultCreated,
		},
		{
			"update existing object",
			[]client.Object{
				&rbacv1.Role{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"fake"},
							Verbs:     []string{"create", "get", "update"},
						},
					},
				},
			},
			controllerutil.OperationResultUpdated,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&rbacv1.Role{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"configmaps"},
							Verbs:     []string{"create", "get", "update"},
						},
						{
							APIGroups: []string{""},
							Resources: []string{"secrets"},
							Verbs:     []string{"create", "get", "update"},
						},
					},
				},
			},
			controllerutil.OperationResultNone,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			rules := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"create", "get", "update"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"create", "get", "update"},
				},
			}

			result, err := CreateOrUpdate(ctx, c,
				&rbacv1.Role{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureRoleRules(rules...))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(result).To(gomega.Equal(tt.result))

			existing := &rbacv1.Role{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.Rules).To(gomega.Equal(rules))
		})
	}
}

func TestEnsureClusterRoleRules(t *testing.T) {
	gomega.RegisterTestingT(t)
	tests := []struct {
		name    string
		objects []client.Object
		result  controllerutil.OperationResult
	}{
		{
			"create new object",
			[]client.Object{},
			controllerutil.OperationResultCreated,
		},
		{
			"update existing object",
			[]client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: v2.ObjectMeta{Name: name},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"fake"},
							Verbs:     []string{"create", "get", "update"},
						},
					},
				},
			},
			controllerutil.OperationResultUpdated,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&rbacv1.ClusterRole{
					ObjectMeta: v2.ObjectMeta{Name: name},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"configmaps"},
							Verbs:     []string{"create", "get", "update"},
						},
						{
							APIGroups: []string{""},
							Resources: []string{"secrets"},
							Verbs:     []string{"create", "get", "update"},
						},
					},
				},
			},
			controllerutil.OperationResultNone,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			rules := []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"create", "get", "update"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"create", "get", "update"},
				},
			}

			result, err := CreateOrUpdate(ctx, c,
				&rbacv1.ClusterRole{ObjectMeta: v2.ObjectMeta{Name: name}},
				EnsureClusterRoleRules(rules...))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(result).To(gomega.Equal(tt.result))

			existing := &rbacv1.ClusterRole{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Name: "test"}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.Rules).To(gomega.Equal(rules))
		})
	}
}
