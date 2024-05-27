//go:build acceptance

package acceptance_tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/e2e/support"
	"strings"
)

var _ = Describe("Trusted Artifact Signer operator images", func() {

	It("(core) are pointing to registry.redhat.io", func() {
		for _, image := range support.GetTasCoreImages() {
			Expect(image).To(ContainSubstring("registry.redhat.io/rhtas"))
		}
	})

	It("(other) are pointing to other Red Hat registry", func() {
		for _, image := range support.GetTasOtherImages() {
			Expect(image).To(Or(ContainSubstring("registry.redhat.io"), ContainSubstring("registry.access.redhat.com")))
		}
	})

	It("are all unique", func() {
		existingImages := make(map[string]struct{})
		for _, image := range support.GetTasImages() {
			if _, ok := existingImages[image]; ok {
				Fail("Not unique image: " + image)
			}
			existingImages[image] = struct{}{}
		}
	})

	It("have hashes as an images tags", func() {
		for _, image := range support.GetTasImages() {
			parts := strings.Split(image, "@sha256:")
			Expect(parts).To(HaveLen(2), "Image tag is not a hash")
			Expect(parts[1]).To(HaveLen(64), "Image SHA has not correct length")
		}
	})
})
