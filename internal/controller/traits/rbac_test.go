package traits

import (
	"testing"

	"github.com/onsi/gomega"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	rbacv1 "k8s.io/api/rbac/v1"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const bindingTestName = "test"

func TestRoleBinding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		objects []client.Object
		result  bool
	}{
		{
			"create new object",
			[]client.Object{},
			true,
		},
		{
			"update existing object",
			[]client.Object{
				&rbacv1.RoleBinding{
					ObjectMeta: v2.ObjectMeta{Name: bindingTestName, Namespace: "default"},
					RoleRef: rbacv1.RoleRef{
						APIGroup: rbacv1.GroupName,
						Kind:     "Role",
						Name:     tufConstants.RBACName,
					},
					Subjects: []rbacv1.Subject{
						{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"},
						{Kind: "ServiceAccount", Name: "fake", Namespace: "default"}},
				},
			},
			true,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&rbacv1.RoleBinding{
					ObjectMeta: v2.ObjectMeta{Name: bindingTestName, Namespace: "default"},
					RoleRef: rbacv1.RoleRef{
						APIGroup: rbacv1.GroupName,
						Kind:     "Role",
						Name:     tufConstants.RBACName,
					},
					Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"}},
				},
			},
			true,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()

			role := rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     tufConstants.RBACName,
			}
			subject := rbacv1.Subject{Kind: "ServiceAccount", Name: tufConstants.RBACName, Namespace: "default"}

			binding := rbacv1ac.RoleBinding(bindingTestName, "default")
			g.Expect(RoleBinding(role, subject)(binding)).To(gomega.Succeed())
			result, err := kubernetes.Apply(ctx, c, binding,
				&rbacv1.RoleBinding{ObjectMeta: v2.ObjectMeta{Name: bindingTestName, Namespace: "default"}})
			g.Expect(err).ToNot(gomega.HaveOccurred())

			g.Expect(result).To(gomega.Equal(tt.result))

			existing := &rbacv1.RoleBinding{}
			g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			g.Expect(existing.RoleRef).To(gomega.Equal(role))
			g.Expect(existing.Subjects).To(gomega.Equal([]rbacv1.Subject{subject}))
		})
	}
}
