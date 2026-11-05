package traits

import (
	"testing"

	"github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestPVCApply(t *testing.T) {
	const name = "test"
	size := resource.MustParse("987Gi")
	pvcSpec := rhtasv1.Pvc{
		Size: &size, StorageClass: "class",
		AccessModes: []rhtasv1.PersistentVolumeAccessMode{rhtasv1.PersistentVolumeAccessMode(corev1.ReadWriteOnce)},
	}
	tests := []struct {
		name     string
		existing *corev1.PersistentVolumeClaim
	}{
		{name: "create new claim"},
		{
			name: "resize claim and preserve externally managed fields",
			existing: &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: name, Namespace: "default",
					Annotations: map[string]string{"pv.kubernetes.io/bind-completed": "yes"},
					Finalizers:  []string{"kubernetes.io/pvc-protection"},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: ptr.To("class"), VolumeName: "bound-volume", VolumeMode: ptr.To(corev1.PersistentVolumeFilesystem),
					Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")}},
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase:    corev1.ClaimBound,
					Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1Gi")},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)
			ctx := t.Context()
			builder := testAction.FakeClientBuilder().WithReturnManagedFields()
			if tt.existing != nil {
				builder.WithObjects(tt.existing)
			}
			cli := builder.Build()
			apply := func() bool {
				desired, err := Apply(corev1ac.PersistentVolumeClaim(name, "default"), PVCSpec(pvcSpec))
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(desired.Status).To(gomega.BeNil())
				changed, err := kubernetes.Apply(ctx, cli, desired, &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
				g.Expect(err).NotTo(gomega.HaveOccurred())
				return changed
			}
			g.Expect(apply()).To(gomega.BeTrue())
			live := &corev1.PersistentVolumeClaim{}
			key := client.ObjectKey{Name: name, Namespace: "default"}
			g.Expect(cli.Get(ctx, key, live)).To(gomega.Succeed())
			g.Expect(live.Spec.AccessModes).To(gomega.Equal([]corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}))
			g.Expect(live.Spec.StorageClassName).To(gomega.Equal(ptr.To("class")))
			g.Expect(live.Spec.Resources.Requests.Storage()).To(gomega.Equal(&size))
			if tt.existing != nil {
				g.Expect(live.Spec.VolumeName).To(gomega.Equal(tt.existing.Spec.VolumeName))
				g.Expect(live.Spec.VolumeMode).To(gomega.Equal(tt.existing.Spec.VolumeMode))
				g.Expect(live.Annotations).To(gomega.Equal(tt.existing.Annotations))
				g.Expect(live.Finalizers).To(gomega.Equal(tt.existing.Finalizers))
				g.Expect(live.Status).To(gomega.Equal(tt.existing.Status))
			}
			owned, err := corev1ac.ExtractPersistentVolumeClaim(live, kubernetes.FieldManager)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(owned.Spec.VolumeName).To(gomega.BeNil())
			g.Expect(owned.Spec.VolumeMode).To(gomega.BeNil())
			g.Expect(owned.Annotations).To(gomega.BeEmpty())
			g.Expect(owned.Finalizers).To(gomega.BeEmpty())
			apply()
			after := &corev1.PersistentVolumeClaim{}
			g.Expect(cli.Get(ctx, key, after)).To(gomega.Succeed())
			// The fake client bumps resourceVersion on every apply.
			live.ResourceVersion, after.ResourceVersion = "", ""
			live.ManagedFields, after.ManagedFields = nil, nil
			g.Expect(after).To(gomega.Equal(live))
		})
	}
}
