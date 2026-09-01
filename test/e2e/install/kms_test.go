//go:build integration

package install

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	fulcioactions "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekoractions "github.com/securesign/operator/internal/controller/rekor/actions"
	tsaactions "github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/openbao"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// parseECDSAPublicKeyPEM decodes a PEM-encoded PKIX public key and asserts it's ECDSA.
func parseECDSAPublicKeyPEM(pemBytes []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	Expect(block).NotTo(BeNil(), "failed to PEM-decode public key: %s", pemBytes)

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecdsaPub, ok := pub.(*ecdsa.PublicKey)
	Expect(ok).To(BeTrue(), "expected ECDSA public key, got %T", pub)
	return ecdsaPub, nil
}

// parseECDSACertPublicKeyPEM decodes a PEM-encoded certificate and returns its ECDSA public key.
func parseECDSACertPublicKeyPEM(pemBytes []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	Expect(block).NotTo(BeNil(), "failed to PEM-decode certificate: %s", pemBytes)

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	ecdsaPub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	Expect(ok).To(BeTrue(), "expected ECDSA public key, got %T", cert.PublicKey)
	return ecdsaPub, nil
}

// findContainer returns the container with the given name from a Deployment, failing the spec if absent.
func findContainer(dep *appsv1.Deployment, name string) *v1.Container {
	for i := range dep.Spec.Template.Spec.Containers {
		if dep.Spec.Template.Spec.Containers[i].Name == name {
			return &dep.Spec.Template.Spec.Containers[i]
		}
	}
	return nil
}

var _ = Describe("Securesign install with KMS (OpenBao) signer", Ordered, func() {
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
		// Deploy an OpenBao dev server and configure the transit keys backing
		// Rekor/Fulcio/TSA's KMS signers.
		Expect(openbao.CreatePrerequisites(ctx, cli, namespace.Name)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		// Fulcio and TSA (unlike Rekor) need a CA certificate chain whose public key
		// matches their OpenBao transit key; generate and store those before creating
		// the Securesign CR, since the reconciler requires the Secret to already exist.
		Expect(openbao.CreateKMSCertificate(ctx, cli, namespace.Name, openbao.FulcioKeyName)).To(Succeed())
		Expect(openbao.CreateKMSTimestampCertificate(ctx, cli, namespace.Name, openbao.TsaKeyName)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.ChooseDefaults(fipsEnabled, namespace.Name),
			securesign.WithKMSOpenBaoSigner(namespace.Name),
			securesign.WithKMSOpenBaoFulcioSigner(namespace.Name),
			securesign.WithKMSOpenBaoTSASigner(namespace.Name),
		)
		Expect(cli.Create(ctx, s)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("KMS (OpenBao) install", func() {
		It("all components reach Ready", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
		})

		It("Rekor server runs with the OpenBao KMS signer", func(ctx SpecContext) {
			dep := &appsv1.Deployment{}
			Expect(cli.Get(ctx, types.NamespacedName{
				Namespace: namespace.Name,
				Name:      rekoractions.ServerDeploymentName,
			}, dep)).To(Succeed())

			container := findContainer(dep, rekoractions.ServerDeploymentName)
			Expect(container).NotTo(BeNil())

			signerFlagIdx := -1
			for i, arg := range container.Args {
				if arg == "--rekor_server.signer" {
					signerFlagIdx = i
					break
				}
			}
			Expect(signerFlagIdx).To(BeNumerically(">=", 0), "container args: %v", container.Args)
			Expect(container.Args[signerFlagIdx+1]).To(Equal("openbao://" + openbao.RekorKeyName))

			Expect(container.Env).To(ContainElements(
				v1.EnvVar{Name: "VAULT_ADDR", Value: openbao.Addr(namespace.Name)},
				v1.EnvVar{Name: "VAULT_TOKEN", Value: openbao.RootToken},
			))
		})

		It("Fulcio server runs with the OpenBao KMS signer", func(ctx SpecContext) {
			dep := &appsv1.Deployment{}
			Expect(cli.Get(ctx, types.NamespacedName{
				Namespace: namespace.Name,
				Name:      fulcioactions.DeploymentName,
			}, dep)).To(Succeed())

			container := findContainer(dep, fulcioactions.DeploymentName)
			Expect(container).NotTo(BeNil())

			Expect(container.Args).To(ContainElement("--ca=kmsca"))

			kmsResourceIdx := -1
			certChainPathIdx := -1
			for i, arg := range container.Args {
				switch arg {
				case "--kms-resource":
					kmsResourceIdx = i
				case "--kms-cert-chain-path":
					certChainPathIdx = i
				}
			}
			Expect(kmsResourceIdx).To(BeNumerically(">=", 0), "container args: %v", container.Args)
			Expect(container.Args[kmsResourceIdx+1]).To(Equal("openbao://" + openbao.FulcioKeyName))
			Expect(certChainPathIdx).To(BeNumerically(">=", 0), "container args: %v", container.Args)
			Expect(container.Args[certChainPathIdx+1]).To(Equal("/var/run/fulcio-secrets/cert.pem"))

			Expect(container.Env).To(ContainElements(
				v1.EnvVar{Name: "VAULT_ADDR", Value: openbao.Addr(namespace.Name)},
				v1.EnvVar{Name: "VAULT_TOKEN", Value: openbao.RootToken},
			))
		})

		It("TSA server runs with the OpenBao KMS signer", func(ctx SpecContext) {
			dep := &appsv1.Deployment{}
			Expect(cli.Get(ctx, types.NamespacedName{
				Namespace: namespace.Name,
				Name:      tsaactions.DeploymentName,
			}, dep)).To(Succeed())

			container := findContainer(dep, tsaactions.DeploymentName)
			Expect(container).NotTo(BeNil())

			Expect(container.Command).To(ContainElements(
				"--timestamp-signer=kms",
				"--kms-key-resource=openbao://"+openbao.TsaKeyName,
			))

			Expect(container.Env).To(ContainElements(
				v1.EnvVar{Name: "VAULT_ADDR", Value: openbao.Addr(namespace.Name)},
				v1.EnvVar{Name: "VAULT_TOKEN", Value: openbao.RootToken},
			))
		})

		It("Rekor's active signing key matches the OpenBao transit key", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Status.RekorStatus.Url+"/api/v1/log/publicKey", nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			rekorPEM, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			vaultPEM, err := openbao.TransitPublicKeyPEM(ctx, namespace.Name, openbao.RekorKeyName)
			Expect(err).NotTo(HaveOccurred())

			rekorKey, err := parseECDSAPublicKeyPEM(rekorPEM)
			Expect(err).NotTo(HaveOccurred())
			vaultKey, err := parseECDSAPublicKeyPEM([]byte(vaultPEM))
			Expect(err).NotTo(HaveOccurred())

			Expect(rekorKey.Equal(vaultKey)).To(BeTrue(),
				"Rekor's active signing key does not match OpenBao transit key %q", openbao.RekorKeyName)
		})

		It("Fulcio's root certificate matches the OpenBao transit key", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Status.FulcioStatus.Url+"/api/v1/rootCert", nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			fulcioPEM, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			vaultPEM, err := openbao.TransitPublicKeyPEM(ctx, namespace.Name, openbao.FulcioKeyName)
			Expect(err).NotTo(HaveOccurred())

			fulcioKey, err := parseECDSACertPublicKeyPEM(fulcioPEM)
			Expect(err).NotTo(HaveOccurred())
			vaultKey, err := parseECDSAPublicKeyPEM([]byte(vaultPEM))
			Expect(err).NotTo(HaveOccurred())

			Expect(fulcioKey.Equal(vaultKey)).To(BeTrue(),
				"Fulcio's root certificate does not match OpenBao transit key %q", openbao.FulcioKeyName)
		})

		It("TSA's leaf certificate matches the OpenBao transit key", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Status.TSAStatus.Url+"/certchain", nil)
			Expect(err).NotTo(HaveOccurred())
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			tsaChainPEM, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			vaultPEM, err := openbao.TransitPublicKeyPEM(ctx, namespace.Name, openbao.TsaKeyName)
			Expect(err).NotTo(HaveOccurred())

			// The chain is leaf-first (see CreateKMSTimestampCertificate), and
			// parseECDSACertPublicKeyPEM only decodes the first PEM block, so this
			// reads the leaf's public key, not the throwaway local root's.
			tsaLeafKey, err := parseECDSACertPublicKeyPEM(tsaChainPEM)
			Expect(err).NotTo(HaveOccurred())
			vaultKey, err := parseECDSAPublicKeyPEM([]byte(vaultPEM))
			Expect(err).NotTo(HaveOccurred())

			Expect(tsaLeafKey.Equal(vaultKey)).To(BeTrue(),
				"TSA's leaf certificate does not match OpenBao transit key %q", openbao.TsaKeyName)
		})

		It("SecureSign CR reaches Ready state", func(ctx SpecContext) {
			securesign.Verify(ctx, cli, namespace.Name, s.Name)
		})

		It("cosign sign and verify", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName,
				s.Status.TufStatus.Url,
				s.Status.FulcioStatus.Url,
				s.Status.RekorStatus.Url,
				s.Status.TSAStatus.Url,
			)
		})
	})
})
