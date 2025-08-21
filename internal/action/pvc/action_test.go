package pvc

import (
	"context"
	"reflect"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestHandle(t *testing.T) {
	const namespace = "default"
	const pvcNameFormat = "pvc"
	const storageClass = "default"
	var nnObj = types.NamespacedName{Name: "test", Namespace: namespace}

	type pre struct {
		before    func(context.Context, gomega.Gomega, client.WithWatch)
		mutateObj func(*v1alpha1.Rekor)
	}
	type want struct {
		result *action.Result
		verify func(context.Context, gomega.Gomega, client.WithWatch)
	}
	tests := []struct {
		name string
		pre  pre
		want want
	}{
		{
			name: "bring your own PVC",
			pre: pre{
				mutateObj: func(obj *v1alpha1.Rekor) {
					obj.Spec.Pvc.Name = "byo-pvc"
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, &v1.PersistentVolumeClaim{})).
						To(gomega.WithTransform(apierrors.IsNotFound, gomega.BeTrue()))
					obj := &v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal("byo-pvc"))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionTrue),
						"Reason": gomega.Equal(ReasonSpecified),
					})))
				},
			},
		},
		{
			name: "create a new PVC",
			pre: pre{
				mutateObj: func(obj *v1alpha1.Rekor) {
					size := resource.MustParse("1Gi")
					obj.Spec.Pvc.Size = &size
					obj.Spec.Pvc.AccessModes = []v1alpha1.PersistentVolumeAccessMode{
						v1alpha1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Pvc.StorageClass = storageClass
					obj.Spec.Pvc.Retain = ptr.To(true)
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal(pvcNameFormat))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionTrue),
						"Reason": gomega.Equal(ReasonCreated),
					})))
				},
			},
		},
		{
			name: "failure missing PVC size",
			pre: pre{
				mutateObj: func(obj *v1alpha1.Rekor) {
					obj.Spec.Pvc.AccessModes = []v1alpha1.PersistentVolumeAccessMode{
						v1alpha1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Pvc.StorageClass = storageClass
				},
			},
			want: want{
				result: testAction.Error(reconcile.TerminalError(ErrPVCSizeNotSet)),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, &v1.PersistentVolumeClaim{})).
						To(gomega.WithTransform(apierrors.IsNotFound, gomega.BeTrue()))

					obj := &v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal(""))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionFalse),
						"Reason": gomega.Equal(constants.Failure),
					})))
				},
			},
		},
		{
			name: "resize a PVC",
			pre: pre{
				before: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      pvcNameFormat,
							Namespace: namespace,
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{
								v1.ReadWriteOnce,
							},
							StorageClassName: ptr.To(storageClass),
							Resources: v1.VolumeResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceStorage: resource.MustParse("100Mi"),
								},
							},
						},
					}
					g.Expect(c.Create(ctx, pvc, &client.CreateOptions{})).To(gomega.Succeed())
				},
				mutateObj: func(obj *v1alpha1.Rekor) {
					size := resource.MustParse("1Gi")
					obj.Spec.Pvc.Size = &size
					obj.Spec.Pvc.AccessModes = []v1alpha1.PersistentVolumeAccessMode{
						v1alpha1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Pvc.StorageClass = storageClass
					obj.Spec.Pvc.Retain = ptr.To(true)
					obj.Status.PvcName = pvcNameFormat
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal(pvcNameFormat))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionTrue),
						"Reason": gomega.Equal(ReasonUpdated),
					})))
				},
			},
		},
		{
			name: "bind existing default PVC",
			pre: pre{
				before: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      pvcNameFormat,
							Namespace: namespace,
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{
								v1.ReadWriteOnce,
							},
							StorageClassName: ptr.To(storageClass),
							Resources: v1.VolumeResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}
					g.Expect(c.Create(ctx, pvc, &client.CreateOptions{})).To(gomega.Succeed())
				},
				mutateObj: func(obj *v1alpha1.Rekor) {},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal(pvcNameFormat))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionTrue),
						"Reason": gomega.Equal(ReasonDiscovered),
					})))
				},
			},
		},
		{
			name: "not changes",
			pre: pre{
				before: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      pvcNameFormat,
							Namespace: namespace,
							Labels:    labels.For("component", "deployment", "test"),
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{
								v1.ReadWriteOnce,
							},
							StorageClassName: ptr.To(storageClass),
							Resources: v1.VolumeResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					}
					g.Expect(c.Create(ctx, pvc, &client.CreateOptions{})).To(gomega.Succeed())
				},
				mutateObj: func(obj *v1alpha1.Rekor) {
					size := resource.MustParse("1Gi")
					obj.Spec.Pvc.Size = &size
					obj.Spec.Pvc.AccessModes = []v1alpha1.PersistentVolumeAccessMode{
						v1alpha1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Pvc.StorageClass = storageClass
					obj.Spec.Pvc.Retain = ptr.To(true)
					obj.Status.PvcName = pvcNameFormat
				},
			},
			want: want{
				result: testAction.StatusUpdate(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &v1alpha1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal(pvcNameFormat))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionTrue),
						"Reason": gomega.Equal(constants.Ready),
					})))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := gomega.NewGomegaWithT(t)
			instance := &v1alpha1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			}

			tt.pre.mutateObj(instance)

			c := testAction.FakeClientBuilder().
				WithObjects(instance).
				WithStatusSubresource(instance).
				Build()
			w := Wrapper[*v1alpha1.Rekor](
				func(r *v1alpha1.Rekor) v1alpha1.Pvc {
					return r.Spec.Pvc
				},
				func(r *v1alpha1.Rekor) string {
					return r.Status.PvcName
				},
				func(r *v1alpha1.Rekor, s string) {
					r.Status.PvcName = s
				},
				func(r *v1alpha1.Rekor) bool {
					return true
				},
			)
			if tt.pre.before != nil {
				tt.pre.before(ctx, g, c)
			}
			a := testAction.PrepareAction(c, NewAction[*v1alpha1.Rekor]("pvc", "component", "deployment", w))
			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}
			tt.want.verify(ctx, g, c)
		})
	}
}

func TestCanHandle(t *testing.T) {
	type args struct {
		condition  *metav1.Condition
		enabled    bool
		pvc        v1alpha1.Pvc
		statusPvc  string
		generation int64
	}
	tests := []struct {
		name      string
		args      args
		canHandle bool
	}{
		{
			name:      "disabled",
			canHandle: false,
			args: args{
				enabled: false,
			},
		}, {
			name:      "empty status pvc name",
			canHandle: true,
			args: args{
				enabled:   true,
				statusPvc: "",
			},
		}, {
			name:      "status condition empty",
			canHandle: true,
			args: args{
				enabled:   true,
				statusPvc: "exists",
				condition: nil,
			},
		}, {
			name:      "status condition is false",
			canHandle: true,
			args: args{
				enabled:   true,
				statusPvc: "exists",
				condition: &metav1.Condition{
					Type:   ConditionType,
					Status: metav1.ConditionFalse,
				},
			},
		}, {
			name:      "status condition is true",
			canHandle: false,
			args: args{
				enabled:   true,
				statusPvc: "exists",
				condition: &metav1.Condition{
					Type:   ConditionType,
					Status: metav1.ConditionTrue,
				},
			},
		}, {
			name:      "status condition is Unknown",
			canHandle: true,
			args: args{
				enabled:   true,
				statusPvc: "exists",
				condition: &metav1.Condition{
					Type:   ConditionType,
					Status: metav1.ConditionUnknown,
				},
			},
		}, {
			name:      "status observed generation empty",
			canHandle: true,
			args: args{
				enabled:    true,
				statusPvc:  "exists",
				generation: 1,
				condition: &metav1.Condition{
					Type:   ConditionType,
					Status: metav1.ConditionTrue,
				},
			},
		},
		{
			name:      "status observed generation same",
			canHandle: false,
			args: args{
				enabled:    true,
				statusPvc:  "exists",
				generation: 1,
				condition: &metav1.Condition{
					Type:               ConditionType,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
				},
			},
		},
		{
			name:      "status observed generation different",
			canHandle: true,
			args: args{
				enabled:    true,
				statusPvc:  "exists",
				generation: 2,
				condition: &metav1.Condition{
					Type:               ConditionType,
					Status:             metav1.ConditionTrue,
					ObservedGeneration: 1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := testAction.FakeClientBuilder().Build()
			w := Wrapper[*v1alpha1.Rekor](
				func(r *v1alpha1.Rekor) v1alpha1.Pvc {
					return tt.args.pvc
				},
				func(r *v1alpha1.Rekor) string {
					return tt.args.statusPvc
				},
				func(r *v1alpha1.Rekor, s string) {
					t.Fatal("unexpected execution")
				},
				func(r *v1alpha1.Rekor) bool {
					return tt.args.enabled
				},
			)

			a := testAction.PrepareAction(c, NewAction[*v1alpha1.Rekor]("pvc", "component", "deployment", w))
			instance := v1alpha1.Rekor{}
			instance.SetGeneration(tt.args.generation)
			if tt.args.condition != nil {
				meta.SetStatusCondition(&instance.Status.Conditions, *tt.args.condition)
			}

			if got := a.CanHandle(context.Background(), &instance); !reflect.DeepEqual(got, tt.canHandle) {
				t.Errorf("CanHandle() = %v, want %v", got, tt.canHandle)
			}
		})
	}
}
