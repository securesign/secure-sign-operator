package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestEnsureRoleBinding(t *testing.T) {
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
				&rbacv1.RoleBinding{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					RoleRef: rbacv1.RoleRef{
						APIGroup: v1.SchemeGroupVersion.Group,
						Kind:     "ClusterRole",
						Name:     tufConstants.RBACName,
					},
					Subjects: []rbacv1.Subject{
						{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"},
						{Kind: "ServiceAccount", Name: "fake", Namespace: "default"}},
				},
			},
			controllerutil.OperationResultUpdated,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&rbacv1.RoleBinding{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					RoleRef: rbacv1.RoleRef{
						APIGroup: v1.SchemeGroupVersion.Group,
						Kind:     "Role",
						Name:     tufConstants.RBACName,
					},
					Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"}},
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

			role := rbacv1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     tufConstants.RBACName,
			}
			subject := rbacv1.Subject{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"}

			result, err := CreateOrUpdate(ctx, c,
				&rbacv1.RoleBinding{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsureRoleBinding(role, subject))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(result).To(gomega.Equal(tt.result))

			existing := &rbacv1.RoleBinding{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.RoleRef).To(gomega.Equal(role))
			gomega.Expect(existing.Subjects).To(gomega.Equal([]rbacv1.Subject{subject}))
		})
	}
}

func TestEnsureClusterRoleBinding(t *testing.T) {
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
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: v2.ObjectMeta{Name: name},
					RoleRef: rbacv1.RoleRef{
						APIGroup: v1.SchemeGroupVersion.Group,
						Kind:     "ClusterRole",
						Name:     tufConstants.RBACName,
					},
					Subjects: []rbacv1.Subject{
						{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"},
						{Kind: "ServiceAccount", Name: "fake", Namespace: "default"}},
				},
			},
			controllerutil.OperationResultUpdated,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&rbacv1.ClusterRoleBinding{
					ObjectMeta: v2.ObjectMeta{Name: name},
					RoleRef: rbacv1.RoleRef{
						APIGroup: v1.SchemeGroupVersion.Group,
						Kind:     "Role",
						Name:     tufConstants.RBACName,
					},
					Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"}},
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

			role := rbacv1.RoleRef{
				APIGroup: v1.SchemeGroupVersion.Group,
				Kind:     "Role",
				Name:     tufConstants.RBACName,
			}
			subject := rbacv1.Subject{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"}

			result, err := CreateOrUpdate(ctx, c,
				&rbacv1.ClusterRoleBinding{ObjectMeta: v2.ObjectMeta{Name: name}},
				EnsureClusterRoleBinding(role, subject))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(result).To(gomega.Equal(tt.result))

			existing := &rbacv1.ClusterRoleBinding{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Name: "test"}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.RoleRef).To(gomega.Equal(role))
			gomega.Expect(existing.Subjects).To(gomega.Equal([]rbacv1.Subject{subject}))
		})
	}
}
