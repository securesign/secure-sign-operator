//go:build integration

package e2e

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

const cliServerNs = "trusted-artifact-signer"

var _ = Describe("CliServer", Ordered, func() {
	var (
		cli        ctrl.Client
		httpClient *http.Client
		url        string
	)

	BeforeAll(func() {
		cli, _ = support.CreateClient()
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Transport: tr, Timeout: 2 * time.Minute}
	})

	Describe("HTTP service", func() {
		It("is available", func(ctx SpecContext) {
			lst := &v1.IngressList{}
			Expect(cli.List(ctx, lst, ctrl.InNamespace(cliServerNs))).To(Succeed())
			Expect(lst.Items).To(HaveLen(1))
			protocol := "http://"
			if len(lst.Items[0].Spec.TLS) > 0 {
				protocol = "https://"
			}
			url = protocol + lst.Items[0].Spec.Rules[0].Host
			testUrl := url + "/clients/"

			Eventually(func(g Gomega) {
				resp, err := httpClient.Get(testUrl)
				g.Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
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
				// currently we are distributing tuftool only for Linux amd64
				if cli == "tuftool" && (!strings.Contains(path, "linux") || !strings.Contains(path, "amd64")) {
					continue
				}

				resp, err := httpClient.Head(fmt.Sprintf(url+path, cli))
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(200), fmt.Sprintf("Client for %s on %s not found", cli, path))
			}
		},
		Entry("cosing", "cosign"),
		Entry("rekor-cli", "rekor-cli"),
		Entry("gitsign", "gitsign"),
		Entry("ec", "ec"),
		Entry("fetch-tsa-certs", "fetch-tsa-certs"),
		Entry("tuftool", "tuftool"),
		Entry("updatetree", "updatetree"),
		Entry("createtree", "createtree"),
	)
})
