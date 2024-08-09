package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestResolveTree_CanHandle(t *testing.T) {
	tests := []struct {
		name         string
		phase        string
		canHandle    bool
		treeID       *int64
		statusTreeID *int64
	}{
		{
			name:      "spec.treeID is not nil and status.treeID is nil",
			phase:     constants.Creating,
			canHandle: true,
			treeID:    ptr.To(int64(123456)),
		},
		{
			name:         "spec.treeID != status.treeID",
			phase:        constants.Creating,
			canHandle:    true,
			treeID:       ptr.To(int64(123456)),
			statusTreeID: ptr.To(int64(654321)),
		},
		{
			name:         "spec.treeID is nil and status.treeID is not nil",
			phase:        constants.Creating,
			canHandle:    false,
			statusTreeID: ptr.To(int64(654321)),
		},
		{
			name:      "spec.treeID is nil and status.treeID is nil",
			phase:     constants.Creating,
			canHandle: true,
		},
		{
			name:      "no phase condition",
			phase:     "",
			canHandle: false,
		},
		{
			name:      constants.Ready,
			phase:     constants.Ready,
			canHandle: true,
		},
		{
			name:      constants.Pending,
			phase:     constants.Pending,
			canHandle: false,
		},
		{
			name:      constants.Creating,
			phase:     constants.Creating,
			canHandle: true,
		},
		{
			name:      constants.Initialize,
			phase:     constants.Initialize,
			canHandle: false,
		},
		{
			name:      constants.Failure,
			phase:     constants.Failure,
			canHandle: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewResolveTreeAction())
			instance := rhtasv1alpha1.Rekor{
				Spec: rhtasv1alpha1.RekorSpec{
					TreeID: tt.treeID,
				},
				Status: rhtasv1alpha1.RekorStatus{
					TreeID: tt.statusTreeID,
				},
			}
			if tt.phase != "" {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   constants.Ready,
					Reason: tt.phase,
				})
			}

			if got := a.CanHandle(context.TODO(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}

func TestResolveTree_Handle(t *testing.T) {
	g := NewWithT(t)
	type env struct {
		spec         rhtasv1alpha1.RekorSpec
		statusTreeId *int64
		configMap    *v1.ConfigMap
	}
	type want struct {
		result *action.Result
		verify func(Gomega, *rhtasv1alpha1.Rekor)
	}
	tests := []struct {
		name string
		env  env
		want want
	}{
		{
			name: "create a new tree",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					TreeID:   nil,
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
				configMap: &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rekor-tree-id-config",
						Namespace: "default",
					},
					Data: map[string]string{
						"tree_id": "5555555",
					},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, rekor *rhtasv1alpha1.Rekor) {
					g.Expect(rekor.Spec.TreeID).Should(BeNil())
					g.Expect(rekor.Status.TreeID).ShouldNot(BeNil())
					g.Expect(rekor.Status.TreeID).To(HaveValue(BeNumerically(">", 0)))
					g.Expect(rekor.Status.TreeID).To(HaveValue(BeNumerically("==", 5555555)))
				},
			},
		},
		{
			name: "update tree from spec",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					TreeID:   ptr.To(int64(123456)),
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
				statusTreeId: ptr.To(int64(654321)),
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, rekor *rhtasv1alpha1.Rekor) {
					g.Expect(rekor.Spec.TreeID).ShouldNot(BeNil())
					g.Expect(rekor.Status.TreeID).ShouldNot(BeNil())
					g.Expect(rekor.Spec.TreeID).To(HaveValue(BeNumerically(">", 0)))
					g.Expect(rekor.Spec.TreeID).To(HaveValue(BeNumerically("==", *rekor.Status.TreeID)))
				},
			},
		},
		{
			name: "use tree from spec",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					TreeID:   ptr.To(int64(123456)),
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(g Gomega, rekor *rhtasv1alpha1.Rekor) {
					g.Expect(rekor.Spec.TreeID).ShouldNot(BeNil())
					g.Expect(rekor.Status.TreeID).ShouldNot(BeNil())
					g.Expect(rekor.Spec.TreeID).To(HaveValue(BeNumerically(">", 0)))
					g.Expect(rekor.Spec.TreeID).To(HaveValue(BeNumerically("==", *rekor.Status.TreeID)))
					g.Expect(rekor.Status.TreeID).To(HaveValue(BeNumerically("==", 123456)))
				},
			},
		},
		{
			name: "ConfigMap data is empty",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					TreeID:   nil,
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
				configMap: &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rekor-tree-id-config",
						Namespace: "default",
					},
					Data: map[string]string{},
				},
			},
			want: want{
				result: testAction.Failed(fmt.Errorf("ConfigMap data is empty")),
				verify: func(g Gomega, rekor *rhtasv1alpha1.Rekor) {
					g.Expect(rekor.Spec.TreeID).Should(BeNil())
					g.Expect(rekor.Status.TreeID).Should(BeNil())
				},
			},
		},
		{
			name: "ConfigMap not found",
			env: env{
				spec: rhtasv1alpha1.RekorSpec{
					Trillian: rhtasv1alpha1.TrillianService{Port: ptr.To(int32(8091))},
				},
			},
			want: want{
				result: testAction.Failed(fmt.Errorf("timed out waiting for the ConfigMap: configmap not found")),
				verify: func(g Gomega, rekor *rhtasv1alpha1.Rekor) {
					g.Expect(rekor.Status.Conditions).To(ContainElement(
						WithTransform(func(c metav1.Condition) string { return c.Message }, ContainSubstring("timed out waiting for the ConfigMap:")),
					))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			instance := &rhtasv1alpha1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rekor",
					Namespace: "default",
				},
				Spec: tt.env.spec,
				Status: rhtasv1alpha1.RekorStatus{
					TreeID: tt.env.statusTreeId,
					Conditions: []metav1.Condition{
						{
							Type:   constants.Ready,
							Reason: constants.Creating,
						},
					},
				},
			}

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()

			if tt.env.configMap != nil {
				err := c.Create(ctx, tt.env.configMap)
				if err != nil {
					t.Fatalf("failed to create config map: %v", err)
				}
			}

			a := testAction.PrepareAction(c, NewResolveTreeAction(func(a *resolveTreeAction) {
				a.timeout = 5 * time.Second // Reduced timeout for testing
			}))

			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want.result)
			}
			if tt.want.verify != nil {
				tt.want.verify(g, instance)
			}
		})
	}
}
