//go:build integration

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

const cliServerNs = "trusted-artifact-signer"

var _ = Describe("CliServer", Ordered, func() {
	var (
		cli        ctrl.Client
		httpClient *http.Client
		url        string
		ctx        = context.TODO()
	)

	BeforeAll(func() {
		cli, _ = CreateClient()
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Transport: tr, Timeout: 2 * time.Minute}
	})

	Describe("HTTP service", func() {
		It("is available", func() {
			lst := &v1.IngressList{}
			Expect(cli.List(ctx, lst, ctrl.InNamespace(cliServerNs))).To(Succeed())
			Expect(len(lst.Items)).To(Equal(1))
			protocol := "http://"
			if len(lst.Items[0].Spec.TLS) > 0 {
				protocol = "https://"
			}
			url = protocol + lst.Items[0].Spec.Rules[0].Host
			testUrl := url + "/clients/"

			Eventually(func(g Gomega) {
				resp, err := httpClient.Get(testUrl)
				g.Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()
				g.Expect(resp.StatusCode).To(Equal(200), fmt.Sprintf("http server is not available at %s", testUrl))
			}).WithTimeout(1 * time.Minute).Should(Succeed())
		})
	})

	DescribeTable("downloadable artifacts",
		func(cli string) {
			for _, path := range []string{
				"/clients/linux/%s-amd64.gz",
				"/clients/linux/%s-arm64.gz",
				"/clients/linux/%s-ppc64le.gz",
				"/clients/linux/%s-s390x.gz",
				"/clients/darwin/%s-amd64.gz",
				"/clients/darwin/%s-arm64.gz",
				"/clients/windows/%s-amd64.gz",
			} {
				resp, err := httpClient.Head(fmt.Sprintf(url+path, cli))
				Expect(err).ToNot(HaveOccurred())
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(200), fmt.Sprintf("Client for %s on %s not found", cli, path))
			}
		},
		Entry("cosing", "cosign"),
		Entry("rekor-cli", "rekor-cli"),
		Entry("gitsign", "gitsign"),
		Entry("ec", "ec"),
	)
})
