package pvc

import (
	"context"
	"reflect"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
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
		mutateObj func(*rhtasv1.Rekor)
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
				mutateObj: func(obj *rhtasv1.Rekor) {
					obj.Spec.Attestations.Pvc.Name = "byo-pvc"
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, &v1.PersistentVolumeClaim{})).
						To(gomega.WithTransform(apierrors.IsNotFound, gomega.BeTrue()))
					obj := &rhtasv1.Rekor{}
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
				mutateObj: func(obj *rhtasv1.Rekor) {
					size := resource.MustParse("1Gi")
					obj.Spec.Attestations.Pvc.Size = &size
					obj.Spec.Attestations.Pvc.AccessModes = []rhtasv1.PersistentVolumeAccessMode{
						rhtasv1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Attestations.Pvc.StorageClass = storageClass
					obj.Spec.Attestations.Pvc.Retain = ptr.To(true)
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &rhtasv1.Rekor{}
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
				mutateObj: func(obj *rhtasv1.Rekor) {
					obj.Spec.Attestations.Pvc.AccessModes = []rhtasv1.PersistentVolumeAccessMode{
						rhtasv1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Attestations.Pvc.StorageClass = storageClass
				},
			},
			want: want{
				result: testAction.Error(reconcile.TerminalError(ErrPVCSizeNotSet)),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, &v1.PersistentVolumeClaim{})).
						To(gomega.WithTransform(apierrors.IsNotFound, gomega.BeTrue()))

					obj := &rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal(""))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionFalse),
						"Reason": gomega.Equal(state.Failure.String()),
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
				mutateObj: func(obj *rhtasv1.Rekor) {
					size := resource.MustParse("1Gi")
					obj.Spec.Attestations.Pvc.Size = &size
					obj.Spec.Attestations.Pvc.AccessModes = []rhtasv1.PersistentVolumeAccessMode{
						rhtasv1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Attestations.Pvc.StorageClass = storageClass
					obj.Spec.Attestations.Pvc.Retain = ptr.To(true)
					obj.Status.PvcName = pvcNameFormat
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &rhtasv1.Rekor{}
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
				mutateObj: func(obj *rhtasv1.Rekor) {},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &rhtasv1.Rekor{}
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
				mutateObj: func(obj *rhtasv1.Rekor) {
					size := resource.MustParse("1Gi")
					obj.Spec.Attestations.Pvc.Size = &size
					obj.Spec.Attestations.Pvc.AccessModes = []rhtasv1.PersistentVolumeAccessMode{
						rhtasv1.PersistentVolumeAccessMode(v1.ReadWriteOnce),
					}
					obj.Spec.Attestations.Pvc.StorageClass = storageClass
					obj.Spec.Attestations.Pvc.Retain = ptr.To(true)
					obj.Status.PvcName = pvcNameFormat
				},
			},
			want: want{
				result: testAction.Return(),
				verify: func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
					pvc := &v1.PersistentVolumeClaim{}
					g.Expect(c.Get(ctx, types.NamespacedName{Name: pvcNameFormat, Namespace: namespace}, pvc)).
						To(gomega.Succeed())
					g.Expect(pvc.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(storageClass)))
					g.Expect(pvc.Spec.Resources.Requests.Storage()).To(gstruct.PointTo(gomega.Equal(resource.MustParse("1Gi"))))
					g.Expect(pvc.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}))

					obj := &rhtasv1.Rekor{}
					g.Expect(c.Get(ctx, nnObj, obj)).To(gomega.Succeed())
					g.Expect(obj.Status.PvcName).To(gomega.Equal(pvcNameFormat))

					con := meta.FindStatusCondition(obj.GetConditions(), ConditionType)
					g.Expect(con).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"Status": gomega.Equal(metav1.ConditionTrue),
					})))
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := gomega.NewGomegaWithT(t)
			instance := &rhtasv1.Rekor{
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
			w := Wrapper[*rhtasv1.Rekor](
				func(r *rhtasv1.Rekor) rhtasv1.Pvc {
					return r.Spec.Attestations.Pvc
				},
				func(r *rhtasv1.Rekor) string {
					return r.Status.PvcName
				},
				func(r *rhtasv1.Rekor, s string) {
					r.Status.PvcName = s
				},
				func(r *rhtasv1.Rekor) bool {
					return true
				},
			)
			if tt.pre.before != nil {
				tt.pre.before(ctx, g, c)
			}
			a := testAction.PrepareAction(kubernetes.WithFieldManager(c, kubernetes.FieldManager), NewAction[*rhtasv1.Rekor]("pvc", "component", "deployment", w))
			if got := a.Handle(ctx, instance); !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Handle() = %v, want %v", got, tt.want.result)
			}
			tt.want.verify(ctx, g, c)
		})
	}
}

func TestHandlePVCImmutableFields(t *testing.T) {
	tests := []struct {
		name         string
		storageClass *string
		accessModes  []v1.PersistentVolumeAccessMode
		requested    rhtasv1.Pvc
		manager      string
		missing      bool
	}{
		{
			name:         "adding a class after the cluster assigned a default",
			storageClass: ptr.To("cluster-default"),
			accessModes:  []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			requested:    rhtasv1.Pvc{StorageClass: "new-class", AccessModes: []rhtasv1.PersistentVolumeAccessMode{"ReadWriteOnce"}},
			manager:      "kube-controller-manager",
		},
		{
			name:         "changing previously owned immutable fields",
			storageClass: ptr.To("original"),
			accessModes:  []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			requested:    rhtasv1.Pvc{StorageClass: "new-class", AccessModes: []rhtasv1.PersistentVolumeAccessMode{"ReadWriteMany"}},
		},
		{
			name:         "omitting previously owned immutable fields",
			storageClass: ptr.To("original"),
			accessModes:  []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
		},
		{
			name:        "preserving an unset storage class",
			accessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			requested:   rhtasv1.Pvc{StorageClass: "new-class"},
		},
		{
			name:         "preserving an explicitly empty storage class",
			storageClass: ptr.To(""),
			accessModes:  []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			requested:    rhtasv1.Pvc{StorageClass: "new-class"},
		},
		{
			name:         "preserving access modes when the CR order changes",
			storageClass: ptr.To("original"),
			accessModes:  []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce, v1.ReadWriteMany},
			requested:    rhtasv1.Pvc{AccessModes: []rhtasv1.PersistentVolumeAccessMode{"ReadWriteMany", "ReadWriteOnce"}},
		},
		{
			name:      "recreating a missing PVC using the CR settings",
			requested: rhtasv1.Pvc{StorageClass: "new-class", AccessModes: []rhtasv1.PersistentVolumeAccessMode{"ReadWriteMany"}},
			missing:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			ctx := t.Context()
			size := resource.MustParse("2Gi")
			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", Generation: 1},
				Status:     rhtasv1.RekorStatus{PvcName: "pvc"},
			}
			instance.Spec.Attestations.Pvc = *tt.requested.DeepCopy()
			instance.Spec.Attestations.Pvc.Size = &size
			instance.Spec.Attestations.Pvc.Retain = ptr.To(true)
			requested := instance.Spec.Attestations.Pvc.DeepCopy()
			builder := testAction.FakeClientBuilder().WithObjects(instance).
				WithStatusSubresource(instance).WithReturnManagedFields()
			var originalPVC *v1.PersistentVolumeClaim
			// Seed the existing PVC with binding metadata and status.
			if !tt.missing {
				originalPVC = &v1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pvc", Namespace: "default",
						Annotations: map[string]string{"pv.kubernetes.io/bind-completed": "yes"},
						Finalizers:  []string{"kubernetes.io/pvc-protection"},
					},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: tt.accessModes, StorageClassName: tt.storageClass,
						VolumeName: "bound-volume", VolumeMode: ptr.To(v1.PersistentVolumeFilesystem),
						Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						}},
					},
					Status: v1.PersistentVolumeClaimStatus{
						Phase:    v1.ClaimBound,
						Capacity: v1.ResourceList{v1.ResourceStorage: resource.MustParse("1Gi")},
					},
				}
				builder.WithObjects(originalPVC)
			}
			cli := builder.Build()
			// Record SSA ownership of the tested fields under the selected manager.
			if !tt.missing {
				spec := corev1ac.PersistentVolumeClaimSpec().WithAccessModes(tt.accessModes...).
					WithResources(corev1ac.VolumeResourceRequirements().WithRequests(v1.ResourceList{
						v1.ResourceStorage: resource.MustParse("1Gi"),
					}))
				spec.StorageClassName = tt.storageClass
				manager := tt.manager
				if manager == "" {
					manager = kubernetes.FieldManager
				}
				g.Expect(cli.Apply(ctx, corev1ac.PersistentVolumeClaim("pvc", "default").WithSpec(spec),
					client.FieldOwner(manager))).To(gomega.Succeed())
			}
			wrapper := Wrapper[*rhtasv1.Rekor](
				func(r *rhtasv1.Rekor) rhtasv1.Pvc { return r.Spec.Attestations.Pvc },
				func(r *rhtasv1.Rekor) string { return r.Status.PvcName },
				func(r *rhtasv1.Rekor, name string) { r.Status.PvcName = name },
				func(*rhtasv1.Rekor) bool { return true },
			)
			a := testAction.PrepareAction(kubernetes.WithFieldManager(cli, kubernetes.FieldManager), NewAction[*rhtasv1.Rekor]("pvc", "component", "deployment", wrapper))
			for range 2 {
				if result := a.Handle(ctx, instance); result != nil {
					g.Expect(result.Err).NotTo(gomega.HaveOccurred())
				}
				live := &v1.PersistentVolumeClaim{}
				g.Expect(cli.Get(ctx, types.NamespacedName{Name: "pvc", Namespace: "default"}, live)).To(gomega.Succeed())
				if tt.missing {
					g.Expect(live.Spec.StorageClassName).To(gomega.Equal(ptr.To(requested.StorageClass)))
					g.Expect(live.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteMany}))
				} else {
					g.Expect(live.Spec.StorageClassName).To(gomega.Equal(tt.storageClass))
					g.Expect(live.Spec.AccessModes).To(gomega.Equal(tt.accessModes))
					g.Expect(live.Spec.VolumeName).To(gomega.Equal(originalPVC.Spec.VolumeName))
					g.Expect(live.Spec.VolumeMode).To(gomega.Equal(originalPVC.Spec.VolumeMode))
					g.Expect(live.Annotations).To(gomega.Equal(originalPVC.Annotations))
					g.Expect(live.Finalizers).To(gomega.Equal(originalPVC.Finalizers))
					g.Expect(live.Status).To(gomega.Equal(originalPVC.Status))
				}
				owned, err := corev1ac.ExtractPersistentVolumeClaim(live, kubernetes.FieldManager)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(owned.Spec.VolumeName).To(gomega.BeNil())
				g.Expect(owned.Spec.VolumeMode).To(gomega.BeNil())
				g.Expect(owned.Annotations).To(gomega.BeEmpty())
				g.Expect(owned.Finalizers).To(gomega.BeEmpty())
				g.Expect(live.Spec.Resources.Requests.Storage().Cmp(size)).To(gomega.BeZero())
				g.Expect(instance.Spec.Attestations.Pvc).To(gomega.Equal(*requested))
				g.Expect(a.CanHandle(ctx, instance)).To(gomega.BeFalse())
			}
		})
	}
}

func TestCanHandle(t *testing.T) {
	type args struct {
		condition  *metav1.Condition
		enabled    bool
		pvc        rhtasv1.Pvc
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
			w := Wrapper[*rhtasv1.Rekor](
				func(r *rhtasv1.Rekor) rhtasv1.Pvc {
					return tt.args.pvc
				},
				func(r *rhtasv1.Rekor) string {
					return tt.args.statusPvc
				},
				func(r *rhtasv1.Rekor, s string) {
					t.Fatal("unexpected execution")
				},
				func(r *rhtasv1.Rekor) bool {
					return tt.args.enabled
				},
			)

			a := testAction.PrepareAction(c, NewAction[*rhtasv1.Rekor]("pvc", "component", "deployment", w))
			instance := rhtasv1.Rekor{}
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
