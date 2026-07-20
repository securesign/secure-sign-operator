//go:build integration

package install

import (
	"bytes"

	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
)

var _ = Describe("Securesign key autodiscovery test", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *rhtasv1.Securesign
	var fipsEnabled bool

	BeforeAll(steps.DetectAndConfigureFIPS(cli, func(enabled bool) {
		fipsEnabled = enabled
	}))

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		if fipsEnabled {
			Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-password")).To(Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func(ctx SpecContext) {
		providedCerts := securesign.WithProvidedEncryptedCerts()
		if fipsEnabled {
			providedCerts = securesign.WithProvidedUnencryptedCerts()
		}
		s = securesign.Create(namespace.Name, "test",
			securesign.ChooseDefaults(fipsEnabled, namespace.Name),
			providedCerts,
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with provided certificates", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, ctlog.CreateSecret(namespace.Name, "my-ctlog-secret", !fipsEnabled))).To(Succeed())
			Expect(cli.Create(ctx, fulcio.CreateSecret(namespace.Name, "my-fulcio-secret", !fipsEnabled))).To(Succeed())
			Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, "my-rekor-secret", !fipsEnabled))).To(Succeed())
			Expect(cli.Create(ctx, tsa.CreateSecrets(namespace.Name, "test-tsa-secret", !fipsEnabled))).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
		})

		It("Verify TUF keys", func(ctx SpecContext) {
			t := tuf.Get(ctx, cli, namespace.Name, s.Name)
			Expect(t).ToNot(BeNil())
			Expect(t.Status.Keys).To(HaveEach(WithTransform(func(k rhtasv1.TufKeyStatus) string { return k.SecretRef.Name }, Not(BeEmpty()))))
			var (
				expected, actual []byte
				err              error
			)
			for _, k := range t.Status.Keys {
				actual, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, k.SecretRef)
				Expect(err).To(Not(HaveOccurred()))

				switch k.Name {
				case "fulcio_v1.crt.pem":
					expected, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, s.Spec.Fulcio.Certificate.CARef)
					Expect(err).To(Not(HaveOccurred()))
				case "rekor.pub":
					expectedKeyRef := s.Spec.Rekor.Signer.KeyRef.DeepCopy()
					expectedKeyRef.Key = "public"
					expected, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
				case "ctfe.pub":
					expectedKeyRef := s.Spec.Ctlog.PrivateKeyRef.DeepCopy()
					expectedKeyRef.Key = "public"
					expected, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
				case "tsa.certchain.pem":
					expectedKeyRef := s.Spec.TimestampAuthority.Signer.CertificateChain.CertificateChainRef.DeepCopy()
					expectedKeyRef.Key = "certificateChain"
					expected, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
				}
				Expect(bytes.TrimSpace(actual)).To(Equal(bytes.TrimSpace(expected)))
			}
		})

		It("Use cosign cli", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})
})
