package kubernetes

import (
	"context"
	"testing"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	testAction "github.com/securesign/operator/internal/testing/action"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const name = "test"

func TestEnsurePVCSpec(t *testing.T) {
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
				&v1.PersistentVolumeClaim{
					ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"},
					Spec: v1.PersistentVolumeClaimSpec{
						AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteMany, v1.ReadWriteOnce},
						Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
							v1.ResourceStorage: resource.MustParse("1Gi"),
						}},
						StorageClassName: utils.Pointer("fake"),
					},
				},
			},
			controllerutil.OperationResultUpdated,
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
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			c := testAction.FakeClientBuilder().
				WithObjects(tt.objects...).
				Build()
			storage := resource.MustParse("987Gi")
			mode := v1.ReadWriteOnce
			pvc := v1alpha1.Pvc{
				Size:         &storage,
				Retain:       utils.Pointer(true),
				Name:         "test",
				StorageClass: "class",
				AccessModes:  []v1alpha1.PersistentVolumeAccessMode{v1alpha1.PersistentVolumeAccessMode(mode)},
			}

			result, err := CreateOrUpdate(ctx, c,
				&v1.PersistentVolumeClaim{ObjectMeta: v2.ObjectMeta{Name: name, Namespace: "default"}},
				EnsurePVCSpec(pvc))
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Expect(result).To(gomega.Equal(tt.result))

			existing := &v1.PersistentVolumeClaim{}
			gomega.Expect(c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test"}, existing)).To(gomega.Succeed())
			gomega.Expect(existing.Spec.AccessModes).To(gomega.Equal([]v1.PersistentVolumeAccessMode{mode}))
			gomega.Expect(existing.Spec.Resources.Requests.Storage()).To(gomega.Equal(pvc.Size))
			gomega.Expect(*existing.Spec.StorageClassName).To(gomega.Equal(pvc.StorageClass))
		})
	}
}
