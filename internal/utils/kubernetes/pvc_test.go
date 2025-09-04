package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/api/v1alpha1"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const name = "test"

func TestEnsurePVCSpec(t *testing.T) {
	storage := resource.MustParse("987Gi")
	mode := v1.ReadWriteOnce
	pvc := v1alpha1.Pvc{
		Size:         &storage,
		Retain:       utils.Pointer(true),
		Name:         "test",
		StorageClass: "class",
		AccessModes:  []v1alpha1.PersistentVolumeAccessMode{v1alpha1.PersistentVolumeAccessMode(mode)},
	}

	verify := func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
		existing := &v1.PersistentVolumeClaim{}
		g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
		g.Expect(existing.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{mode}))
		g.Expect(existing.Spec.Resources.Requests.Storage()).To(gomega.Equal(pvc.Size))
		g.Expect(existing.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal(pvc.StorageClass)))
	}
	tests := []struct {
		name    string
		objects []client.Object
		result  controllerutil.OperationResult
		verify  func(context.Context, gomega.Gomega, client.WithWatch)
	}{
		{
			"create new object",
			[]client.Object{},
			controllerutil.OperationResultCreated,
			verify,
		},
		{
			"update existing object",
			[]client.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						}},
						StorageClassName: utils.Pointer("class"),
					},
				},
			},
			controllerutil.OperationResultUpdated,
			verify,
		},
		{
			"do not update immutable fields",
			[]client.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany, v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("987Gi"),
						}},
						StorageClassName: utils.Pointer("test"),
					},
				},
			},
			controllerutil.OperationResultNone,
			func(ctx context.Context, g gomega.Gomega, c client.WithWatch) {
				existing := &v1.PersistentVolumeClaim{}
				g.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
				g.Expect(existing.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{v1.ReadWriteMany, v1.ReadWriteOnce}))
				g.Expect(existing.Spec.Resources.Requests.Storage()).To(gomega.Equal(pvc.Size))
				g.Expect(existing.Spec.StorageClassName).To(gstruct.PointTo(gomega.Equal("test")))
			},
		},
		{
			"set storage class only when nil",
			[]client.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("987Gi"),
						}},
						StorageClassName: nil,
					},
				},
			},
			controllerutil.OperationResultUpdated,
			verify,
		},
		{
			"set access mode only when empty",
			[]client.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{},
						Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("987Gi"),
						}},
						StorageClassName: utils.Pointer("class"),
					},
				},
			},
			controllerutil.OperationResultUpdated,
			verify,
		},
		{
			"existing object with expected values",
			[]client.Object{
				&v1.PersistentVolumeClaim{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("987Gi"),
						}},
						StorageClassName: utils.Pointer("class"),
					},
				},
			},
			controllerutil.OperationResultNone,
			verify,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			g := gomega.NewWithT(t)
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()
			result, err := CreateOrUpdate(ctx, c,
				&v1.PersistentVolumeClaim{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsurePVCSpec(pvc))
			g.Expect(err).ToNot(gomega.HaveOccurred())

			g.Expect(result).To(gomega.Equal(tt.result))

			tt.verify(ctx, g, c)
		})
	}
}
