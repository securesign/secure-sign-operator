//go:build ha

package ha

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

var _ = Describe("PKCS#11 HA Securesign install", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign
	var replicas *int32

	BeforeAll(func() {
		SetDefaultEventuallyTimeout(6 * time.Minute)
	})

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		replicas = ptr.To(int32(2))
		s = securesign.Create(namespace.Name, "test",
			securesign.WithTSA(),
			securesign.WithPKCS11Certs(),
			securesign.WithPKCS11Persistence(),
			securesign.WithManagedDatabase(),
			securesign.WithExternalAccess(),
			securesign.WithDefaultOIDC(),
			securesign.WithNTPMonitoring(),
			securesign.WithReplicas(replicas),
			securesign.WithNFSPVC(),
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with PKCS#11 CA and HA configured", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Fulcio should have the correct replica count", func(ctx SpecContext) {
			fulcio.Verify(ctx, cli, namespace.Name, s.Name)
			Eventually(func(ctx SpecContext) (int32, error) {
				var dep appsv1.Deployment
				if err := cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: fulcioactions.DeploymentName}, &dep); err != nil {
					return 0, err
				}
				return dep.Status.AvailableReplicas, nil
			}).WithContext(ctx).Should(BeNumerically(">=", *replicas), "fulcio should have at least %d available replicas", *replicas)
		})

		It("Fulcio service has ready endpoints", func(ctx SpecContext) {
			Eventually(kubernetes.ExpectServiceHasAtLeastNReadyEndpoints).
				WithContext(ctx).
				WithArguments(cli, namespace.Name, fulcioactions.DeploymentName, 2).
				Should(Succeed(), "expected fulcio service to have 2 ready endpoints")
		})

		It("Use cosign cli", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})

		It("Fulcio remains functional when a pod is deleted", func(ctx SpecContext) {
			kubernetes.RemainsFunctionalWhenOnePodDeleted(ctx, cli, namespace.Name, fulcioactions.DeploymentName, func() {
				s = securesign.Get(ctx, cli, namespace.Name, s.Name)
				tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
			})
		})
	})
})
