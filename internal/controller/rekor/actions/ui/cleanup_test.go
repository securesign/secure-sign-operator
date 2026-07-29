package ui

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/testing/action"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCleanupAction_CanHandle(t *testing.T) {
	tests := []struct {
		name     string
		instance *rhtasv1.Rekor
		want     bool
	}{
		{
			name: "no UiAvailable condition",
			instance: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			},
			want: false,
		},
		{
			name: "has UiAvailable condition",
			instance: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				Status: rhtasv1.RekorStatus{
					Conditions: []metav1.Condition{
						{
							Type:   actions.UICondition,
							Status: metav1.ConditionTrue,
							Reason: "Ready",
						},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			a := NewCleanupAction()
			g.Expect(a.CanHandle(t.Context(), tt.instance)).To(Equal(tt.want))
		})
	}
}

func TestCleanupAction_Handle(t *testing.T) {
	tests := []struct {
		name             string
		ownedResources   []client.Object
		unownedResources []client.Object
		wantDeleted      int
	}{
		{
			name:        "deletes owned resources and removes condition",
			wantDeleted: 4,
			ownedResources: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: actions.SearchUiDeploymentName, Namespace: "default"},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: actions.SearchUiDeploymentName, Namespace: "default"},
				},
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: actions.RBACUIName, Namespace: "default"},
				},
				&rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{Name: actions.RBACUIName, Namespace: "default"},
				},
			},
		},
		{
			name:        "no resources exist — only removes condition",
			wantDeleted: 0,
		},
		{
			name: "skips resources not owned by this Rekor",
			unownedResources: []client.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: actions.SearchUiDeploymentName, Namespace: "default"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					UID:       "test-uid",
				},
				Status: rhtasv1.RekorStatus{
					Conditions: []metav1.Condition{
						{
							Type:   actions.UICondition,
							Status: metav1.ConditionTrue,
							Reason: "Ready",
						},
					},
				},
			}

			objects := []client.Object{instance}
			for _, r := range tt.ownedResources {
				r.SetOwnerReferences([]metav1.OwnerReference{
					{
						APIVersion:         rhtasv1.GroupVersion.String(),
						Kind:               "Rekor",
						Name:               instance.Name,
						UID:                instance.UID,
						Controller:         ptr.To(true),
						BlockOwnerDeletion: ptr.To(true),
					},
				})
				objects = append(objects, r)
			}
			objects = append(objects, tt.unownedResources...)

			cli := action.FakeClientBuilder().WithObjects(objects...).WithStatusSubresource(&rhtasv1.Rekor{}).Build()
			a := action.PrepareAction(cli, NewCleanupAction())

			result := a.Handle(t.Context(), instance)
			g.Expect(result).ToNot(BeNil())
			g.Expect(result.Err).ToNot(HaveOccurred())

			g.Expect(meta.FindStatusCondition(instance.Status.Conditions, actions.UICondition)).To(BeNil())

			deleted := 0
			for _, r := range tt.ownedResources {
				err := cli.Get(t.Context(), client.ObjectKeyFromObject(r), r)
				if errors.IsNotFound(err) {
					deleted++
				}
			}
			g.Expect(deleted).To(Equal(tt.wantDeleted))

			for _, r := range tt.unownedResources {
				g.Expect(cli.Get(t.Context(), client.ObjectKeyFromObject(r), r)).To(Succeed())
			}
		})
	}
}
