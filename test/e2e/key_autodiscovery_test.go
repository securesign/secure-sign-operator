//go:build integration

package e2e

import (
	"context"

	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("Securesign key autodiscovery test", Ordered, func() {
	cli, _ := support.CreateClient()
	ctx := context.TODO()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			_ = cli.Delete(ctx, namespace)
		})

		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithProvidedCerts(),
		)
	})

	BeforeAll(func() {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with provided certificates", func() {
		BeforeAll(func() {
			Expect(cli.Create(ctx, ctlog.CreateSecret(namespace.Name, "my-ctlog-secret"))).To(Succeed())
			Expect(cli.Create(ctx, fulcio.CreateSecret(namespace.Name, "my-fulcio-secret"))).To(Succeed())
			Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, "my-rekor-secret"))).To(Succeed())
			Expect(cli.Create(ctx, tsa.CreateSecrets(namespace.Name, "test-tsa-secret"))).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func() {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		It("Verify TUF keys", func() {
			t := tuf.Get(ctx, cli, namespace.Name, s.Name)()
			Expect(t).ToNot(BeNil())
			Expect(t.Status.Keys).To(HaveEach(WithTransform(func(k v1alpha1.TufKey) string { return k.SecretRef.Name }, Not(BeEmpty()))))
			var (
				expected, actual []byte
				err              error
			)
			for _, k := range t.Status.Keys {
				actual, err = kubernetes.GetSecretData(cli, namespace.Name, k.SecretRef)
				Expect(err).To(Not(HaveOccurred()))

				switch k.Name {
				case "fulcio_v1.crt.pem":
					expected, err = kubernetes.GetSecretData(cli, namespace.Name, s.Spec.Fulcio.Certificate.CARef)
					Expect(err).To(Not(HaveOccurred()))
				case "rekor.pub":
					expectedKeyRef := s.Spec.Rekor.Signer.KeyRef.DeepCopy()
					expectedKeyRef.Key = "public"
					expected, err = kubernetes.GetSecretData(cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
				case "ctfe.pub":
					expectedKeyRef := s.Spec.Ctlog.PrivateKeyRef.DeepCopy()
					expectedKeyRef.Key = "public"
					expected, err = kubernetes.GetSecretData(cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
				case "tsa.certchain.pem":
					expectedKeyRef := s.Spec.TimestampAuthority.Signer.CertificateChain.CertificateChainRef.DeepCopy()
					expectedKeyRef.Key = "certificateChain"
					expected, err = kubernetes.GetSecretData(cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
				}
				Expect(expected).To(Equal(actual))
			}
		})

		It("Use cosign cli", func() {
			tas.VerifyByCosign(ctx, cli, s, targetImageName)
		})
	})
})
