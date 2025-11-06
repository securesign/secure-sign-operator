package rbac

import (
	"context"
	"reflect"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type namedTest struct {
	name string
	run  func(t *testing.T)
}

var tests []namedTest

var (
	nnObject = types.NamespacedName{Name: "test", Namespace: "default"}
)

func init() {
	tests = []namedTest{
		{name: "handle", run: testHandle},
		{name: "serviceAccount", run: testServiceAccount},
		{name: "role", run: testRole},
		{name: "roleBinding", run: testRoleBinding},
	}
}

type pre struct {
	warmUp bool
	opts   []func(*rbacAction[*v1alpha1.Rekor])
	before func(context.Context, Gomega, client.WithWatch)
}
type want struct {
	result *action.Result
	verify func(context.Context, Gomega, client.WithWatch)
}

func testHandle(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "create empty rules",
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					err := c.Get(ctx, nnObject, &corev1.ServiceAccount{})
					g.Expect(err).To(Succeed())

					err = c.Get(ctx, nnObject, &rbacv1.Role{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))

					err = c.Get(ctx, nnObject, &rbacv1.RoleBinding{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))
				},
			},
		},
		{
			desc: "when empty rules, delete Role and RoleBinding",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					g.Expect(c.Create(ctx, &corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
					})).To(Succeed())

					g.Expect(c.Create(ctx, &rbacv1.Role{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
						Rules: []rbacv1.PolicyRule{},
					})).To(Succeed())

					g.Expect(c.Create(ctx, &rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
					})).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					err := c.Get(ctx, nnObject, &corev1.ServiceAccount{})
					g.Expect(err).To(Succeed())

					err = c.Get(ctx, nnObject, &rbacv1.Role{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))

					err = c.Get(ctx, nnObject, &rbacv1.RoleBinding{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))
				},
			},
		},
		{
			desc: "create with rules",
			pre: pre{
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithRule[*v1alpha1.Rekor](rbacv1.PolicyRule{
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())

					r := rbacv1.Role{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					g.Expect(r.Rules).To(HaveLen(1))
					g.Expect(r.Rules[0]).To(Equal(rbacv1.PolicyRule{
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}))

					rb := rbacv1.RoleBinding{}
					g.Expect(c.Get(ctx, nnObject, &rb)).To(Succeed())
					g.Expect(rb.RoleRef).To(Equal(rbacv1.RoleRef{
						APIGroup: corev1.SchemeGroupVersion.Group,
						Kind:     "Role",
						Name:     r.Name,
					}))
					g.Expect(rb.Subjects).To(HaveLen(1))
					g.Expect(rb.Subjects[0]).To(Equal(rbacv1.Subject{
						APIGroup:  corev1.SchemeGroupVersion.Group,
						Kind:      "ServiceAccount",
						Name:      sa.Name,
						Namespace: sa.Namespace,
					}))
				},
			},
		},
		{
			desc: "update rules",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {

					g.Expect(c.Create(ctx, &corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
					})).To(Succeed())

					g.Expect(c.Create(ctx, &rbacv1.Role{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
						Rules: []rbacv1.PolicyRule{
							{
								Resources: []string{"secrets"},
								Verbs:     []string{"list", "watch"},
							},
						},
					})).To(Succeed())

					g.Expect(c.Create(ctx, &rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
					})).To(Succeed())
				},
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithRule[*v1alpha1.Rekor](rbacv1.PolicyRule{
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())

					r := rbacv1.Role{}
					g.Expect(c.Get(ctx, nnObject, &r)).To(Succeed())
					g.Expect(r.Rules).To(HaveLen(1))
					g.Expect(r.Rules[0]).To(Equal(rbacv1.PolicyRule{
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}))

					rb := rbacv1.RoleBinding{}
					g.Expect(c.Get(ctx, nnObject, &rb)).To(Succeed())
					g.Expect(rb.RoleRef).To(Equal(rbacv1.RoleRef{
						APIGroup: corev1.SchemeGroupVersion.Group,
						Kind:     "Role",
						Name:     r.Name,
					}))
					g.Expect(rb.Subjects).To(HaveLen(1))
					g.Expect(rb.Subjects[0]).To(Equal(rbacv1.Subject{
						APIGroup:  corev1.SchemeGroupVersion.Group,
						Kind:      "ServiceAccount",
						Name:      sa.Name,
						Namespace: sa.Namespace,
					}))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *rbacAction[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.Handle(ctx, rekor)
		}))
	}
}

func testServiceAccount(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "create",
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())
				},
			},
		},
		{
			desc: "update",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
					}
					g.Expect(c.Create(ctx, &sa)).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())

					g.Expect(sa.Labels).To(Not(BeEmpty()))
					g.Expect(sa.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "component"))
				},
			},
		},
		{
			desc: "create with ImagePullSecrets",
			pre: pre{
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithImagePullSecrets[*v1alpha1.Rekor](func(instance *v1alpha1.Rekor) []corev1.LocalObjectReference {
						return []corev1.LocalObjectReference{
							{Name: "pull-secret-1"},
							{Name: "pull-secret-2"},
						}
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())
					g.Expect(sa.ImagePullSecrets).To(HaveLen(2))
					g.Expect(sa.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "pull-secret-1"}))
					g.Expect(sa.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "pull-secret-2"}))
				},
			},
		},
		{
			desc: "create without ImagePullSecrets when function returns nil",
			pre: pre{
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithImagePullSecrets[*v1alpha1.Rekor](func(instance *v1alpha1.Rekor) []corev1.LocalObjectReference {
						return nil
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())
					g.Expect(sa.ImagePullSecrets).To(BeEmpty())
				},
			},
		},
		{
			desc: "create without ImagePullSecrets when function returns empty list",
			pre: pre{
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithImagePullSecrets[*v1alpha1.Rekor](func(instance *v1alpha1.Rekor) []corev1.LocalObjectReference {
						return []corev1.LocalObjectReference{}
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())
					g.Expect(sa.ImagePullSecrets).To(BeEmpty())
				},
			},
		},
		{
			desc: "update ServiceAccount with ImagePullSecrets",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
						ImagePullSecrets: []corev1.LocalObjectReference{
							{Name: "old-secret"},
						},
					}
					g.Expect(c.Create(ctx, &sa)).To(Succeed())
				},
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithImagePullSecrets[*v1alpha1.Rekor](func(instance *v1alpha1.Rekor) []corev1.LocalObjectReference {
						return []corev1.LocalObjectReference{
							{Name: "new-secret"},
						}
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := corev1.ServiceAccount{}
					g.Expect(c.Get(ctx, nnObject, &sa)).To(Succeed())
					g.Expect(sa.ImagePullSecrets).To(HaveLen(1))
					g.Expect(sa.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "new-secret"}))
					g.Expect(sa.ImagePullSecrets).ToNot(ContainElement(corev1.LocalObjectReference{Name: "old-secret"}))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *rbacAction[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleServiceAccount(ctx, rekor)
		}))
	}
}

func testRole(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "do not create when rules are not set",
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					err := c.Get(ctx, nnObject, &rbacv1.Role{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))
				},
			},
		},
		{
			desc: "delete existing object when rules are not set",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					g.Expect(c.Create(ctx, &rbacv1.Role{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
						Rules: []rbacv1.PolicyRule{},
					})).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					err := c.Get(ctx, nnObject, &rbacv1.Role{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))
				},
			},
		},
		{
			desc: "create",
			pre: pre{
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithRule[*v1alpha1.Rekor](rbacv1.PolicyRule{
						APIGroups: []string{""},
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					role := rbacv1.Role{}
					g.Expect(c.Get(ctx, nnObject, &role)).To(Succeed())
					g.Expect(role.Rules).To(HaveLen(1))
				},
			},
		},
		{
			desc: "update rules",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := rbacv1.Role{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
						Rules: []rbacv1.PolicyRule{
							{
								Resources: []string{"secrets"},
								Verbs:     []string{"list", "watch"},
							},
						},
					}
					g.Expect(c.Create(ctx, &sa)).To(Succeed())
				},
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithRule[*v1alpha1.Rekor](rbacv1.PolicyRule{
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					role := rbacv1.Role{}
					g.Expect(c.Get(ctx, nnObject, &role)).To(Succeed())

					g.Expect(role.Labels).To(Not(BeEmpty()))
					g.Expect(role.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "component"))
					g.Expect(role.Rules).To(HaveLen(1))
					g.Expect(role.Rules[0]).To(Equal(rbacv1.PolicyRule{
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *rbacAction[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleRole(ctx, rekor)
		}))
	}
}

func testRoleBinding(t *testing.T) {
	for _, tc := range []struct {
		desc string
		pre  pre
		want want
	}{
		{
			desc: "do not create when rules are not set",
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					err := c.Get(ctx, nnObject, &rbacv1.RoleBinding{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))
				},
			},
		},
		{
			desc: "delete existing object when rules are not set",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					g.Expect(c.Create(ctx, &rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
					})).To(Succeed())
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					err := c.Get(ctx, nnObject, &rbacv1.RoleBinding{})
					g.Expect(err).To(HaveOccurred())
					g.Expect(err).To(WithTransform(errors.IsNotFound, BeTrue()))
				},
			},
		},
		{
			desc: "create",
			pre: pre{
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithRule[*v1alpha1.Rekor](rbacv1.PolicyRule{
						APIGroups: []string{""},
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					rb := rbacv1.RoleBinding{}
					g.Expect(c.Get(ctx, nnObject, &rb)).To(Succeed())
					g.Expect(rb.RoleRef).To(Equal(rbacv1.RoleRef{
						APIGroup: corev1.SchemeGroupVersion.Group,
						Kind:     "Role",
						Name:     nnObject.Name,
					}))
					g.Expect(rb.Subjects).To(HaveLen(1))
				},
			},
		},
		{
			desc: "update rules",
			pre: pre{
				before: func(ctx context.Context, g Gomega, c client.WithWatch) {
					sa := rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      nnObject.Name,
							Namespace: nnObject.Namespace,
						},
					}
					g.Expect(c.Create(ctx, &sa)).To(Succeed())
				},
				opts: []func(action2 *rbacAction[*v1alpha1.Rekor]){
					WithRule[*v1alpha1.Rekor](rbacv1.PolicyRule{
						Resources: []string{"configmaps"},
						Verbs:     []string{"list", "watch"},
					}),
				},
			},
			want: want{
				result: testAction.Continue(),
				verify: func(ctx context.Context, g Gomega, c client.WithWatch) {
					rb := rbacv1.RoleBinding{}
					g.Expect(c.Get(ctx, nnObject, &rb)).To(Succeed())

					g.Expect(rb.Labels).To(Not(BeEmpty()))
					g.Expect(rb.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "component"))
					g.Expect(rb.Subjects).To(HaveLen(1))
				},
			},
		},
	} {
		t.Run(tc.desc, testRunner(tc.pre, tc.want, func(r *rbacAction[*v1alpha1.Rekor], ctx context.Context, rekor *v1alpha1.Rekor) *action.Result {
			return r.handleRoleBinding(ctx, rekor)
		}))
	}
}

type handleFn func(*rbacAction[*v1alpha1.Rekor], context.Context, *v1alpha1.Rekor) *action.Result

func testRunner(pre pre, want want, handleFn handleFn) func(t *testing.T) {
	return func(t *testing.T) {
		g := NewWithT(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		instance := &v1alpha1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nnObject.Name,
				Namespace: nnObject.Namespace,
			},
			Spec: v1alpha1.RekorSpec{
				Trillian: v1alpha1.TrillianService{
					Address: "trillian-logserver",
					Port:    ptr.To(int32(8091)),
				},
			},
		}

		c := testAction.FakeClientBuilder().
			WithObjects(instance).
			WithStatusSubresource(instance).
			Build()

		a := testAction.PrepareAction(c, NewAction[*v1alpha1.Rekor]("component", nnObject.Name, pre.opts...))
		ra := a.(*rbacAction[*v1alpha1.Rekor])

		if pre.warmUp {
			handleFn(ra, ctx, instance)
		}

		if pre.before != nil {
			pre.before(ctx, g, c)
		}

		g.Expect(c.Get(ctx, nnObject, instance)).To(Succeed())

		if got := handleFn(ra, ctx, instance); !reflect.DeepEqual(got, want.result) {
			t.Errorf("CanHandle() = %v, want %v", got, want.result)
		}
		if want.verify != nil {
			want.verify(ctx, g, c)
		}
	}
}

func TestRbac_CanHandle(t *testing.T) {
	tests := []struct {
		name      string
		reason    string
		opts      []func(*rbacAction[*v1alpha1.Rekor])
		canHandle bool
	}{
		{
			name:      constants.Ready,
			reason:    constants.Ready,
			canHandle: true,
		}, {
			name:      constants.Creating,
			reason:    constants.Creating,
			canHandle: true,
		}, {
			name:      constants.Failure,
			reason:    constants.Failure,
			canHandle: false,
		}, {
			name:      constants.Initialize,
			reason:    constants.Initialize,
			canHandle: false,
		}, {
			name:      constants.Pending,
			reason:    constants.Pending,
			canHandle: false,
		}, {
			name:      "empty status",
			reason:    "",
			canHandle: false,
		}, {
			name:      "custom can handle func: pass",
			reason:    "",
			canHandle: true,
			opts: []func(*rbacAction[*v1alpha1.Rekor]){
				WithCanHandle(func(ctx context.Context, t *v1alpha1.Rekor) bool {
					return true
				}),
			},
		}, {
			name:      "custom can handle func: reject",
			reason:    "",
			canHandle: false,
			opts: []func(*rbacAction[*v1alpha1.Rekor]){
				WithCanHandle(func(ctx context.Context, t *v1alpha1.Rekor) bool {
					return false
				}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewAction[*v1alpha1.Rekor]("component", "test", tt.opts...))
			instance := v1alpha1.Rekor{}
			if tt.reason != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   constants.Ready,
					Reason: tt.reason,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestRbac_Handle(t *testing.T) {
	for _, nt := range tests {
		t.Run(nt.name, func(t *testing.T) {
			nt.run(t)
		})
	}
}
