package traits

import (
	"testing"

	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
)

func TestConvertViaJSON(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	source := core.ResourceRequirements{
		Limits: core.ResourceList{
			core.ResourceCPU:    resource.MustParse("500m"),
			core.ResourceMemory: resource.MustParse("128Mi"),
		},
		Requests: core.ResourceList{
			core.ResourceCPU: resource.MustParse("100m"),
		},
		Claims: []core.ResourceClaim{{Name: "accelerator", Request: "gpu"}},
	}

	converted, err := ConvertViaJSON[core.ResourceRequirements, corev1ac.ResourceRequirementsApplyConfiguration](source)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect((*converted.Limits)[core.ResourceCPU]).To(Equal(resource.MustParse("500m")))
	g.Expect((*converted.Limits)[core.ResourceMemory]).To(Equal(resource.MustParse("128Mi")))
	g.Expect((*converted.Requests)[core.ResourceCPU]).To(Equal(resource.MustParse("100m")))
	g.Expect(converted.Claims).To(HaveLen(1))
	g.Expect(converted.Claims[0].Name).To(HaveValue(Equal("accelerator")))
	g.Expect(converted.Claims[0].Request).To(HaveValue(Equal("gpu")))
}
