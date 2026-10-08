//go:build integration

package install

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	tufAction "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	testKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/cosign"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/openbao"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// limitMigrationAttempts caps retries and supplies a per-attempt deadline to
// operations that honor context cancellation.
func limitMigrationAttempts(operation func(context.Context) error) func(context.Context) error {
	const maxAttempts = 3
	attempts := 0
	return func(ctx context.Context) error {
		if attempts >= maxAttempts {
			return StopTrying("migration operation exhausted its 3 attempts")
		}
		attempts++
		attemptCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		err := operation(attemptCtx)
		if err != nil && attempts == maxAttempts {
			return StopTrying("migration operation failed after 3 attempts").Wrap(err)
		}
		return err
	}
}

// Starts with operator-generated file signers, migrates to KMS, then returns
// to file mode without providing replacement certificate or private-key refs.
// The original file Secrets remain present so the round trip checks both stale
// KMS references (SECURESIGN-5653) and unintended reuse of historical file keys.
var _ = Describe("Securesign file-to-KMS-to-file signer migration", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace   *v1.Namespace
		s           *rhtasv1.Securesign
		fipsEnabled bool

		targetImageV1 string
		targetImageV2 string
		localCosign   *cosign.LocalCosign

		fileEraFulcioRef     string
		fileEraTsaRef        string
		fileEraRekorRef      string
		fileEraFulcioCert    []byte
		fileEraTsaCert       []byte
		fileEraRekorPub      []byte
		fileEraFulcioKeyRef  *rhtasv1.SecretKeySelector
		fileEraTsaKeyRef     *rhtasv1.SecretKeySelector
		fileEraFulcioKeyHash [sha256.Size]byte
		fileEraTsaKeyHash    [sha256.Size]byte

		kmsEraFulcioRef  string
		kmsEraTsaRef     string
		kmsEraFulcioCert []byte
		kmsEraTsaCert    []byte
		kmsEraRekorPub   []byte

		tufRepoWorkdir string
		tufPod         v1.Pod
	)

	BeforeAll(func() {
		if _, err := exec.LookPath("tufcli"); err != nil {
			Skip("tufcli command not found")
		}
	})

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

	// Created up-front even though the instance starts in file mode -- the
	// later migration step needs these secrets to already exist before
	// patching the CR to reference them.
	BeforeAll(func(ctx SpecContext) {
		Expect(openbao.CreatePrerequisites(ctx, cli, namespace.Name)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		Expect(openbao.CreateKMSCertificate(ctx, cli, namespace.Name, openbao.FulcioKeyName)).To(Succeed())
		Expect(openbao.CreateKMSTimestampCertificate(ctx, cli, namespace.Name, openbao.TsaKeyName)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.ChooseDefaults(fipsEnabled, namespace.Name),
		)
		Expect(cli.Create(ctx, s)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageV1 = support.PrepareImage(ctx)
	})

	It("all components reach Ready in file mode", func(ctx SpecContext) {
		tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
	})

	It("signs and verifies an image while running in file mode", func(ctx SpecContext) {
		s = securesign.Get(ctx, cli, namespace.Name, s.Name)
		localCosign = cosign.NewLocalCosign(s.Status.TufStatus.URL, s.Status.FulcioStatus.URL, s.Status.RekorStatus.URL, s.Status.TSAStatus.URL)

		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return clients.Execute("cosign", "initialize", "--mirror="+s.Status.TufStatus.URL, "--root="+s.Status.TufStatus.URL+"/root.json")
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Sign(ctx, targetImageV1)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV1)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})

	It("captures the file-era secret refs and key fingerprints", func(ctx SpecContext) {
		f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
		Expect(f.Status.Certificate).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef).ToNot(BeNil())
		fileEraFulcioRef = f.Status.Certificate.CARef.Name
		var err error
		fileEraFulcioCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(fileEraFulcioCert).ToNot(BeEmpty())
		Expect(f.Status.Certificate.PrivateKeyRef).ToNot(BeNil())
		fileEraFulcioKeyRef = f.Status.Certificate.PrivateKeyRef.DeepCopy()
		fulcioKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraFulcioKeyRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(fulcioKey).ToNot(BeEmpty())
		// Compare fingerprints so failed assertions do not print private keys.
		fileEraFulcioKeyHash = sha256.Sum256(fulcioKey)

		t := tsa.Get(ctx, cli, namespace.Name, s.Name)
		Expect(t.Status.Signer).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef).ToNot(BeNil())
		fileEraTsaRef = t.Status.Signer.CertificateChainRef.Name
		fileEraTsaCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(fileEraTsaCert).ToNot(BeEmpty())
		Expect(t.Status.Signer.FileSigner).ToNot(BeNil())
		Expect(t.Status.Signer.FileSigner.PrivateKeyRef).ToNot(BeNil())
		fileEraTsaKeyRef = t.Status.Signer.FileSigner.PrivateKeyRef.DeepCopy()
		tsaKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraTsaKeyRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(tsaKey).ToNot(BeEmpty())
		fileEraTsaKeyHash = sha256.Sum256(tsaKey)

		r := rekor.Get(ctx, cli, namespace.Name, s.Name)
		Expect(r.Status.Signer.KeyRef).ToNot(BeNil(),
			"Rekor should have a real signer KeyRef while running in file mode")
		fileEraRekorRef = r.Status.Signer.KeyRef.Name
		Expect(r.Status.PublicKey).ToNot(BeEmpty())
		fileEraRekorPub = []byte(r.Status.PublicKey)
	})

	It("migrates Fulcio, TSA and Rekor to KMS mode", func(ctx SpecContext) {
		Eventually(func() error {
			f := securesign.Get(ctx, cli, namespace.Name, s.Name)
			original := f.DeepCopy()

			securesign.WithKMSOpenBaoSigner(namespace.Name)(f)
			securesign.WithKMSOpenBaoFulcioSigner(namespace.Name)(f)
			securesign.WithKMSOpenBaoTSASigner(namespace.Name)(f)

			return cli.Patch(ctx, f, client.MergeFrom(original))
		}).Should(Succeed())
	})

	It("acknowledges trust material drift after migrating to KMS mode", func(ctx SpecContext) {
		Eventually(func(g Gomega) error {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			g.Expect(f).ToNot(BeNil())
			if f.Annotations == nil {
				f.Annotations = map[string]string{}
			}
			f.Annotations[annotations.RefreshTrustMaterial] = "true"
			return cli.Update(ctx, f)
		}).Should(Succeed())

		Eventually(func(g Gomega) error {
			t := tsa.Get(ctx, cli, namespace.Name, s.Name)
			g.Expect(t).ToNot(BeNil())
			if t.Annotations == nil {
				t.Annotations = map[string]string{}
			}
			t.Annotations[annotations.RefreshTrustMaterial] = "true"
			return cli.Update(ctx, t)
		}).Should(Succeed())

		Eventually(func(g Gomega) error {
			r := rekor.Get(ctx, cli, namespace.Name, s.Name)
			g.Expect(r).ToNot(BeNil())
			if r.Annotations == nil {
				r.Annotations = map[string]string{}
			}
			r.Annotations[annotations.RefreshTrustMaterial] = "true"
			return cli.Update(ctx, r)
		}).Should(Succeed())
	})

	It("Fulcio comes back Ready with the KMS-backed cert", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(fulcio.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Fulcio never became Ready after migrating to KMS mode")

		f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
		Expect(f.Status.Certificate).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef.Name).ToNot(Equal(fileEraFulcioRef))
		kmsEraFulcioRef = f.Status.Certificate.CARef.Name
		var err error
		kmsEraFulcioCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(kmsEraFulcioCert).ToNot(BeEmpty())
	})

	It("TSA comes back Ready with the KMS-backed cert", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(tsa.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"TimestampAuthority never became Ready after migrating to KMS mode")

		t := tsa.Get(ctx, cli, namespace.Name, s.Name)
		Expect(t.Status.Signer).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef.Name).ToNot(Equal(fileEraTsaRef))
		kmsEraTsaRef = t.Status.Signer.CertificateChainRef.Name
		var err error
		kmsEraTsaCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(kmsEraTsaCert).ToNot(BeEmpty())
	})

	It("Rekor comes back Ready with the KMS-backed key", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(rekor.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Rekor never became Ready after migrating to KMS mode")

		r := rekor.Get(ctx, cli, namespace.Name, s.Name)
		Expect(r.Status.PublicKey).ToNot(BeEmpty())
		kmsEraRekorPub = []byte(r.Status.PublicKey)
	})

	It("all components remain Ready end-to-end after migrating to KMS mode", func(ctx SpecContext) {
		s = securesign.Get(ctx, cli, namespace.Name, s.Name)
		tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
	})

	Describe("Sync TUF after the file -> KMS migration", func() {
		var certs string

		It("downloads the TUF repository", func(ctx SpecContext) {
			var err error
			certs, err = os.MkdirTemp(os.TempDir(), "certs")
			Expect(err).ToNot(HaveOccurred())

			tufRepoWorkdir, err = os.MkdirTemp(os.TempDir(), "tuf-repo")
			Expect(err).ToNot(HaveOccurred())

			tufKeys := &v1.Secret{}
			Expect(os.Mkdir(filepath.Join(tufRepoWorkdir, "keys"), 0777)).To(Succeed())
			Expect(cli.Get(ctx, client.ObjectKey{Name: "tuf-root-keys", Namespace: namespace.Name}, tufKeys)).To(Succeed())
			for k, v := range tufKeys.Data {
				Expect(os.WriteFile(filepath.Join(tufRepoWorkdir, "keys", k), v, 0644)).To(Succeed())
			}

			Expect(os.Mkdir(filepath.Join(tufRepoWorkdir, "tuf-repo"), 0777)).To(Succeed())
			tufPodList := &v1.PodList{}
			Expect(cli.List(ctx, tufPodList, client.InNamespace(namespace.Name), client.MatchingLabels{labels.LabelAppName: tufAction.DeploymentName})).To(Succeed())
			Expect(tufPodList.Items).To(HaveLen(1))
			tufPod = tufPodList.Items[0]

			Expect(testKubernetes.CopyFromPod(ctx, tufPod, "/var/www/html", filepath.Join(tufRepoWorkdir, "tuf-repo"))).To(Succeed())
		})

		It("resolves service URLs", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			Expect(s.Status.FulcioStatus.URL).ToNot(BeEmpty())
			Expect(s.Status.RekorStatus.URL).ToNot(BeEmpty())
			Expect(s.Status.TufStatus.URL).ToNot(BeEmpty())
			Expect(s.Status.TSAStatus.URL).ToNot(BeEmpty())
		})

		It("syncs the fulcio target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/fulcio_v1.crt.pem", fileEraFulcioCert, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-fulcio-kms.cert.pem", kmsEraFulcioCert, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "fulcio_v1.crt.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "new-fulcio-kms.cert.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the rekor target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/rekor.pub", fileEraRekorPub, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-rekor-kms.pub", kmsEraRekorPub, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "rekor.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "new-rekor-kms.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the tsa target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/tsa.certchain.pem", fileEraTsaCert, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-tsa-kms.certchain.pem", kmsEraTsaCert, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "tsa.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "new-tsa-kms.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("uploads the TUF repository back", func(ctx SpecContext) {
			Expect(testKubernetes.CopyToPod(ctx, config.GetConfigOrDie(), tufPod, filepath.Join(tufRepoWorkdir, "tuf-repo"), "/var/www/html")).To(Succeed())
		})
	})

	It("signs and verifies a KMS-era image, and confirms the file-era image still verifies", func(ctx SpecContext) {
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return clients.Execute("cosign", "initialize", "--mirror="+s.Status.TufStatus.URL, "--root="+s.Status.TufStatus.URL+"/root.json")
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

		targetImageV2 = support.PrepareImage(ctx)
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Sign(ctx, targetImageV2)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV2)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV1)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})

	// This is the actual SECURESIGN-5653 regression path, now exercised from a
	// genuine KMS baseline (migrated into, not freshly deployed into) rather
	// than a same-run KMS install.
	It("reverts Fulcio, TSA and Rekor back to operator-generated file signers", func(ctx SpecContext) {
		Eventually(func() error {
			f := securesign.Get(ctx, cli, namespace.Name, s.Name)
			original := f.DeepCopy()

			f.Spec.Fulcio.Signer = rhtasv1.FulcioSigner{
				CertificateChain: rhtasv1.FulcioCertificateChain{
					OrganizationName: "Red Hat",
				},
			}
			f.Spec.Fulcio.Auth = nil

			f.Spec.TimestampAuthority.Signer = rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{
						{OrganizationName: "Red Hat"},
					},
					LeafCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
				},
			}
			f.Spec.TimestampAuthority.Auth = nil

			f.Spec.Rekor.Signer = rhtasv1.RekorSigner{}
			f.Spec.Rekor.Auth = nil

			return cli.Patch(ctx, f, client.MergeFrom(original))
		}).Should(Succeed())
	})

	It("acknowledges trust material drift after reverting to file mode", func(ctx SpecContext) {
		Eventually(func(g Gomega) error {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			g.Expect(f).ToNot(BeNil())
			if f.Annotations == nil {
				f.Annotations = map[string]string{}
			}
			f.Annotations[annotations.RefreshTrustMaterial] = "true"
			return cli.Update(ctx, f)
		}).Should(Succeed())

		Eventually(func(g Gomega) error {
			t := tsa.Get(ctx, cli, namespace.Name, s.Name)
			g.Expect(t).ToNot(BeNil())
			if t.Annotations == nil {
				t.Annotations = map[string]string{}
			}
			t.Annotations[annotations.RefreshTrustMaterial] = "true"
			return cli.Update(ctx, t)
		}).Should(Succeed())

		Eventually(func(g Gomega) error {
			r := rekor.Get(ctx, cli, namespace.Name, s.Name)
			g.Expect(r).ToNot(BeNil())
			if r.Annotations == nil {
				r.Annotations = map[string]string{}
			}
			r.Annotations[annotations.RefreshTrustMaterial] = "true"
			return cli.Update(ctx, r)
		}).Should(Succeed())
	})

	It("Fulcio returns to Ready with a new file key and certificate while retaining the original Secret", func(ctx SpecContext) {
		Eventually(func(ctx context.Context) (bool, error) {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			if f != nil && (f.Spec.Signer.Type == "" || f.Spec.Signer.Type == rhtasv1.SignerTypeFile) &&
				f.Spec.Signer.Kms == nil && condition.IsReady(f) {
				ready := meta.FindStatusCondition(f.GetConditions(), constants.ReadyCondition)
				if ready.ObservedGeneration == f.GetGeneration() {
					return true, nil
				}
			}
			// Events can outlive a failed rollout; retain them as timeout
			// diagnostics instead of aborting before the controller can recover.
			if msg := failedMountEvent(ctx, cli, namespace.Name, "fulcio-server"); msg != "" {
				return false, fmt.Errorf("Fulcio is not Ready in file mode; recorded FailedMount: %s", msg)
			}
			return false, fmt.Errorf("Fulcio is not Ready in file mode for its current generation")
		}).WithContext(ctx).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Fulcio never became Ready after reverting to file mode")

		f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
		Expect(f.Status.Certificate).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef.Name).ToNot(Equal(kmsEraFulcioRef),
			"Fulcio status still references the old KMS cert-chain secret after reverting to file mode")
		Expect(f.Status.Certificate.CARef.Name).ToNot(Equal(fileEraFulcioRef),
			"Fulcio must not reactivate the original file-era certificate Secret")
		Expect(f.Status.Certificate.PrivateKeyRef).ToNot(BeNil())
		Expect(f.Status.Certificate.PrivateKeyRef.Name).ToNot(Equal(fileEraFulcioKeyRef.Name),
			"Fulcio must generate a new file signer Secret after the KMS round trip")

		privKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.certificate.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(privKey).ToNot(BeEmpty())
		Expect(sha256.Sum256(privKey)).ToNot(Equal(fileEraFulcioKeyHash),
			"Fulcio must generate a new private key, not copy the original key into a new Secret")

		newCert, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(newCert).ToNot(BeEmpty())
		Expect(sha256.Sum256(newCert)).ToNot(Equal(sha256.Sum256(fileEraFulcioCert)),
			"Fulcio must publish a new certificate after rotating its file signer")

		retainedKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraFulcioKeyRef)
		Expect(err).ToNot(HaveOccurred(), "the original Fulcio file signer Secret must be retained")
		Expect(sha256.Sum256(retainedKey)).To(Equal(fileEraFulcioKeyHash),
			"the original Fulcio private key must remain unchanged")
	})

	It("TSA returns to Ready with a new file key and certificate chain while retaining the original Secret", func(ctx SpecContext) {
		Eventually(func(ctx context.Context) (bool, error) {
			t := tsa.Get(ctx, cli, namespace.Name, s.Name)
			if t != nil && (t.Spec.Signer.Type == "" || t.Spec.Signer.Type == rhtasv1.SignerTypeFile) &&
				t.Spec.Signer.Kms == nil && t.Spec.Signer.Tink == nil && condition.IsReady(t) {
				ready := meta.FindStatusCondition(t.GetConditions(), constants.ReadyCondition)
				if ready.ObservedGeneration == t.GetGeneration() {
					return true, nil
				}
			}
			if msg := failedMountEvent(ctx, cli, namespace.Name, "tsa-server"); msg != "" {
				return false, fmt.Errorf("TSA is not Ready in file mode; recorded FailedMount: %s", msg)
			}
			return false, fmt.Errorf("TSA is not Ready in file mode for its current generation")
		}).WithContext(ctx).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"TimestampAuthority never became Ready after reverting to file mode")

		t := tsa.Get(ctx, cli, namespace.Name, s.Name)
		Expect(t.Status.Signer).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef.Name).ToNot(Equal(kmsEraTsaRef),
			"TSA status still references the old KMS cert-chain secret after reverting to file mode")
		Expect(t.Status.Signer.CertificateChainRef.Name).ToNot(Equal(fileEraTsaRef),
			"TSA must not reactivate the original file-era certificate-chain Secret")

		Expect(t.Status.Signer.FileSigner).ToNot(BeNil())
		Expect(t.Status.Signer.FileSigner.PrivateKeyRef).ToNot(BeNil())
		Expect(t.Status.Signer.FileSigner.PrivateKeyRef.Name).ToNot(Equal(fileEraTsaKeyRef.Name),
			"TSA must generate a new file signer Secret after the KMS round trip")
		leafKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.FileSigner.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.signer.fileSigner.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(leafKey).ToNot(BeEmpty())
		Expect(sha256.Sum256(leafKey)).ToNot(Equal(fileEraTsaKeyHash),
			"TSA must generate a new private key, not copy the original key into a new Secret")

		newChain, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(newChain).ToNot(BeEmpty())
		Expect(sha256.Sum256(newChain)).ToNot(Equal(sha256.Sum256(fileEraTsaCert)),
			"TSA must publish a new certificate chain after rotating its file signer")

		retainedKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraTsaKeyRef)
		Expect(err).ToNot(HaveOccurred(), "the original TSA file signer Secret must be retained")
		Expect(sha256.Sum256(retainedKey)).To(Equal(fileEraTsaKeyHash),
			"the original TSA private key must remain unchanged")
	})

	It("Rekor comes back Ready with a freshly generated file secret", func(ctx SpecContext) {
		Eventually(func(ctx context.Context) bool {
			r := rekor.Get(ctx, cli, namespace.Name, s.Name)
			if r != nil && (r.Spec.Signer.Type == "" || r.Spec.Signer.Type == rhtasv1.SignerTypeSecret) &&
				r.Spec.Signer.Kms == nil && condition.IsReady(r) {
				ready := meta.FindStatusCondition(r.GetConditions(), constants.ReadyCondition)
				return ready.ObservedGeneration == r.GetGeneration()
			}
			return false
		}).WithContext(ctx).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Rekor never became Ready after reverting to file mode")

		r := rekor.Get(ctx, cli, namespace.Name, s.Name)
		Expect(r.Status.Signer.KeyRef).ToNot(BeNil())
		Expect(r.Status.Signer.KeyRef.Name).ToNot(Equal(fileEraRekorRef),
			"Rekor should generate a brand new signer secret, not reuse the original file-era one")

		privKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, r.Status.Signer.KeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.signer.keyRef must resolve to a real secret key")
		Expect(privKey).ToNot(BeEmpty())

		oldRef := r.Status.Signer.KeyRef.DeepCopy()
		oldRef.Name = fileEraRekorRef
		oldPrivKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, oldRef)
		Expect(err).ToNot(HaveOccurred(), "the original file signer Secret must be retained")
		Expect(sha256.Sum256(oldPrivKey)).ToNot(Equal(sha256.Sum256(privKey)), "file signer rotation must create a new key pair")
		Expect(r.Status.PublicKey).ToNot(Equal(string(fileEraRekorPub)), "Rekor must publish the new file signer's public key")
	})

	It("all components remain Ready end-to-end after reverting to file mode", func(ctx SpecContext) {
		s = securesign.Get(ctx, cli, namespace.Name, s.Name)
		tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
	})

	Describe("Sync TUF after reverting to file mode", func() {
		It("syncs the fulcio target", func(ctx SpecContext) {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			newFulcioCert, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
			Expect(err).ToNot(HaveOccurred())

			certs, err := os.MkdirTemp(os.TempDir(), "certs")
			Expect(err).ToNot(HaveOccurred())
			Expect(os.WriteFile(certs+"/new-fulcio-kms.cert.pem", kmsEraFulcioCert, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-fulcio-file.cert.pem", newFulcioCert, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "new-fulcio-kms.cert.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "new-fulcio-file.cert.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the rekor target", func(ctx SpecContext) {
			r := rekor.Get(ctx, cli, namespace.Name, s.Name)
			Expect(r.Status.PublicKey).ToNot(BeEmpty())

			certs, err := os.MkdirTemp(os.TempDir(), "certs")
			Expect(err).ToNot(HaveOccurred())
			Expect(os.WriteFile(certs+"/new-rekor-kms.pub", kmsEraRekorPub, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-rekor-file.pub", []byte(r.Status.PublicKey), 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "new-rekor-kms.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "new-rekor-file.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the tsa target", func(ctx SpecContext) {
			t := tsa.Get(ctx, cli, namespace.Name, s.Name)
			newTsaChain, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
			Expect(err).ToNot(HaveOccurred())

			certs, err := os.MkdirTemp(os.TempDir(), "certs")
			Expect(err).ToNot(HaveOccurred())
			Expect(os.WriteFile(certs+"/new-tsa-kms.certchain.pem", kmsEraTsaCert, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-tsa-file.certchain.pem", newTsaChain, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "new-tsa-kms.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "new-tsa-file.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("uploads the TUF repository back", func(ctx SpecContext) {
			Expect(testKubernetes.CopyToPod(ctx, config.GetConfigOrDie(), tufPod, filepath.Join(tufRepoWorkdir, "tuf-repo"), "/var/www/html")).To(Succeed())
		})
	})

	It("signs a third image and confirms all three generations still verify (full backward compatibility)", func(ctx SpecContext) {
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return clients.Execute("cosign", "initialize", "--mirror="+s.Status.TufStatus.URL, "--root="+s.Status.TufStatus.URL+"/root.json")
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

		targetImageV3 := support.PrepareImage(ctx)
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Sign(ctx, targetImageV3)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV3)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV2)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV1)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})
})
