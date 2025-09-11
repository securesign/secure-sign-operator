//go:build ha

package ha

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	ctlogactions "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekoractions "github.com/securesign/operator/internal/controller/rekor/actions"
	trillianactions "github.com/securesign/operator/internal/controller/trillian/actions"
	tsaactions "github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/internal/controller/tuf/constants"
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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

		It("fulcio should have the correct replica count", func(ctx SpecContext) {
			fulcio.Verify(ctx, cli, namespace.Name, s.Name)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: fulcioactions.DeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "fulcio should have at least %d available replicas", *replicas)
		})

		It("rekor server should have the correct replica count", func(ctx SpecContext) {
			rekor.Verify(ctx, cli, namespace.Name, s.Name, true)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: rekoractions.ServerComponentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "rekor server should have at least %d available replicas", *replicas)
		})

		It("rekor search ui should have the correct replica count", func(ctx SpecContext) {
			rekor.VerifySearchUI(ctx, cli, namespace.Name)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: rekoractions.SearchUiDeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "rekor search ui should have at least %d available replicas", *replicas)
		})

		It("ctlog should have the correct replica count", func(ctx SpecContext) {
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: ctlogactions.DeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "ctlog should have at least %d available replicas", *replicas)
		})

		It("tsa should have the correct replica count", func(ctx SpecContext) {
			tsa.Verify(ctx, cli, namespace.Name, s.Name)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: tsaactions.DeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "tsa should have at least %d available replicas", *replicas)
		})

		It("tuf should have the correct replica count", func(ctx SpecContext) {
			tuf.Verify(ctx, cli, namespace.Name, s.Name)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: constants.DeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "tuf should have at least %d available replicas", *replicas)
		})

		It("log server should have the correct replica count", func(ctx SpecContext) {
			trillian.Verify(ctx, cli, namespace.Name, s.Name, true)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: trillianactions.LogserverDeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "log server should have at least %d available replicas", *replicas)
		})

		It("log signer should have the correct replica count", func(ctx SpecContext) {
			trillian.Verify(ctx, cli, namespace.Name, s.Name, true)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: trillianactions.LogsignerDeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "log signer should have at least %d available replicas", *replicas)
		})

		It("Services have ready endpoints", func(ctx SpecContext) {
			endpointNames := []string{ctlogactions.ComponentName, fulcioactions.DeploymentName, rekoractions.SearchUiDeploymentName, rekoractions.ServerComponentName, trillianactions.LogServerComponentName, trillianactions.LogSignerComponentName, tsaactions.DeploymentName, constants.ComponentName}
			for _, endpointName := range endpointNames {
				Eventually(kubernetes.ExpectServiceHasAtLeastNReadyEndpoints).
					WithContext(ctx).
					WithArguments(cli, namespace.Name, endpointName, 2).
					Should(Succeed(), "expected service to have n ready endpoints")
			}
		})

		It("Use cosign cli", func(ctx SpecContext) {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})

		It("ctlog remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, ctlogactions.ComponentName, func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("fulcio remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, fulcioactions.DeploymentName, func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("rekor remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, rekoractions.ServerComponentName, func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("rekor-search-ui remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, rekoractions.SearchUiDeploymentName, func() {
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
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, trillianactions.LogServerComponentName, func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("trillian-signer elects a new leader when a pod is deleted", func(ctx SpecContext) {
			leaderBefore, err := kubernetes.GetLeaseHolderIdentity(ctx, cli, namespace.Name, trillianactions.LogSignerComponentName)
			Expect(err).NotTo(HaveOccurred())

			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, trillianactions.LogSignerComponentName, func() {
				Eventually(func(ctx SpecContext) string {
					leaderAfter, _ := kubernetes.GetLeaseHolderIdentity(
						ctx, cli, namespace.Name, trillianactions.LogSignerComponentName,
					)
					return leaderAfter
				}).WithContext(ctx).ShouldNot(Equal(leaderBefore))
			})

		})
		It("Tsa remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, tsaactions.DeploymentName, func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
		It("TUF remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, constants.ComponentName, func() {
				tas.VerifyByCosign(ctx, cli, s, targetImageName)
			})
		})
	})
})
