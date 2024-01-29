//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

const cliServerNs = "trusted-artifact-signer"

var _ = Describe("CliServer is running", func() {
	cli, _ := CreateClient()
	ctx := context.TODO()

	When("operator is installed ", func() {
		It("is up exposed", func() {
			lst := &v1.IngressList{}
			gomega.Expect(cli.List(ctx, lst, ctrl.InNamespace(cliServerNs))).To(gomega.Succeed())
			gomega.Expect(len(lst.Items)).To(gomega.Equal(1))
			protocol := "http://"
			if len(lst.Items[0].Spec.TLS) > 0 {
				protocol = "https://"
			}
			url := protocol + lst.Items[0].Spec.Rules[0].Host

			for _, c := range []string{"cosign", "rekor-cli", "gitsign"} {
				for _, path := range []string{
					"/clients/linux/%s-amd64.gz",
					"/clients/linux/%s-arm64.gz",
					"/clients/linux/%s-ppc64le.gz",
					"/clients/linux/%s-s390x.gz",
					"/clients/darwin/%s-amd64.gz",
					"/clients/darwin/%s-arm64.gz",
					"/clients/windows/%s-amd64.gz",
				} {
					resp, err := http.Get(fmt.Sprintf(url+path, c))
					gomega.Expect(err).ToNot(gomega.HaveOccurred())
					gomega.Expect(resp.StatusCode).To(gomega.Equal(200))
				}

			}
		})
	})

})
