//go:build ha

package ha

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("HA Securesign install", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign
	var replicas *int32

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		replicas = ptr.To(int32(2))
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithSearchUI(),
			securesign.WithReplicas(replicas),
			securesign.WithNFSPVC(),
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with HA configured", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All other components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("has the correct number of replicas configured for HA", func(ctx SpecContext) {
			Expect(ptr.Deref(fulcio.Get(ctx, cli, namespace.Name, s.Name).Spec.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "fulcio should have more than one replica")
			Expect(ptr.Deref(rekor.Get(ctx, cli, namespace.Name, s.Name).Spec.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "rekor should have more than one replica")
			Expect(ptr.Deref(rekor.Get(ctx, cli, namespace.Name, s.Name).Spec.RekorSearchUI.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "rekor search ui should have more than one replica")
			Expect(ptr.Deref(ctlog.Get(ctx, cli, namespace.Name, s.Name).Spec.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "ctlog should have more than one replica")
			Expect(ptr.Deref(tsa.Get(ctx, cli, namespace.Name, s.Name).Spec.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "tsa should have more than one replica")
			Expect(ptr.Deref(tuf.Get(ctx, cli, namespace.Name, s.Name).Spec.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "tuf should have more than one replica")
			Expect(ptr.Deref(trillian.Get(ctx, cli, namespace.Name, s.Name).Spec.LogServer.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "log server should have more than one replica")
			Expect(ptr.Deref(trillian.Get(ctx, cli, namespace.Name, s.Name).Spec.LogSigner.Replicas, 0)).
				To(BeNumerically(">=", *replicas), "log signer should have more than one replica")
		})

		It("Services have ready endpoints", func(ctx SpecContext) {
			endpointNames := []string{"ctlog", "fulcio-server", "rekor-search-ui", "rekor-server", "trillian-logserver", "trillian-logsigner", "tsa-server", "tuf"}
			for _, endpointName := range endpointNames {
				err := kubernetes.ExpectServiceHasAtLeastNReadyEndpoints(ctx, cli, namespace.Name, endpointName, 2)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Use cosign cli", func(ctx SpecContext) {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})

		It("ctlog remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "ctlog", func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("fulcio remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "fulcio-server", func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("rekor remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "rekor-server", func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("rekor-search-ui remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "rekor-search-ui", func() {
				r := rekor.Get(ctx, cli, namespace.Name, s.Name)
				Expect(r).ToNot(BeNil())
				Expect(r.Status.RekorSearchUIUrl).NotTo(BeEmpty())

				httpClient := http.Client{
					Timeout: time.Second * 10,
				}
				Eventually(func() bool {
					resp, err := httpClient.Get(r.Status.RekorSearchUIUrl)
					if err != nil {
						return false
					}
					defer func() { _ = resp.Body.Close() }()
					return resp.StatusCode == http.StatusOK
				}).Should(BeTrue(), "Rekor UI should be accessible and return a status code of 200")
			})
		})
		It("trillian-logserver remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "trillian-logserver", func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("trillian-signer elects a new leader when a pod is deleted", func(ctx SpecContext) {
			leaderBefore, err := kubernetes.GetCurrentLeader(ctx, cli, namespace.Name, "trillian-logsigner")
			Expect(err).NotTo(HaveOccurred())

			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "trillian-logsigner", func() {
				Eventually(func() string {
					leaderAfter, _ := kubernetes.GetCurrentLeader(ctx, cli, namespace.Name, "trillian-logsigner")
					return leaderAfter
				}).ShouldNot(Equal(leaderBefore))
			})
		})
		It("Tsa remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "tsa-server", func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("TUF remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, "tuf", func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
	})
})
