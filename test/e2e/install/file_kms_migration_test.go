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
	"github.com/securesign/operator/internal/apis"
	fulcioActions "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekorActions "github.com/securesign/operator/internal/controller/rekor/actions"
	tsaActions "github.com/securesign/operator/internal/controller/tsa/actions"
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

// Starts with operator-generated file signers, migrates to KMS, then returns to
// file mode. The Secrets from the first file era are still present throughout,
// which is what makes the return dangerous: the pre-fix operator adopted them by
// name and resumed signing with the superseded key (SECURESIGN-5702), on top of
// the stale KMS refs of SECURESIGN-5653.
//
// The return is attempted twice: with nothing but the KMS settings removed,
// which must be refused, and with the Secrets the customer created.
var _ = Describe("Securesign file-to-KMS-to-file signer migration", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace   *v1.Namespace
		s           *rhtasv1.Securesign
		fipsEnabled bool

		targetImageV1 string
		targetImageV2 string
		tsChainV1     string
		tsChainV2     string
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

		// Customer-created replacement material for the return from KMS.
		newFulcioSecret = "new-fulcio-cert"
		newTsaSecret    = "new-tsa-cert"
		newRekorSecret  = "new-rekor-signer"

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

		tsChainV1 = fmt.Sprintf("ts_chain_%s_v1.pem", namespace.Name)
		Expect(tsa.SaveCertificateChain(ctx, s.Status.TSAStatus.URL, tsChainV1)).To(Succeed())
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
		acknowledgeTrustMaterialDrift(ctx, cli, func() apis.ConditionsAwareObject {
			return fulcio.Get(ctx, cli, namespace.Name, s.Name)
		})

		acknowledgeTrustMaterialDrift(ctx, cli, func() apis.ConditionsAwareObject {
			return tsa.Get(ctx, cli, namespace.Name, s.Name)
		})

		acknowledgeTrustMaterialDrift(ctx, cli, func() apis.ConditionsAwareObject {
			return rekor.Get(ctx, cli, namespace.Name, s.Name)
		})
	})

	It("Fulcio comes back Ready with the KMS-backed cert", func(ctx SpecContext) {
		Eventually(func() bool {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			return f != nil && f.Spec.Signer.Kms != nil && condition.IsReadyForGeneration(f)
		}).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
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
			t := tsa.Get(ctx, cli, namespace.Name, s.Name)
			return t != nil && t.Spec.Signer.Kms != nil && condition.IsReadyForGeneration(t)
		}).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
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
			r := rekor.Get(ctx, cli, namespace.Name, s.Name)
			return r != nil && r.Spec.Signer.Kms != nil && condition.IsReadyForGeneration(r)
		}).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
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

		tsChainV2 = fmt.Sprintf("ts_chain_%s_v2.pem", namespace.Name)
		Expect(tsa.SaveCertificateChain(ctx, s.Status.TSAStatus.URL, tsChainV2)).To(Succeed())

		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.VerifyWithTimestampChain(ctx, targetImageV1, tsChainV1)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})

	// This is the actual SECURESIGN-5653 regression path, now exercised from a
	// genuine KMS baseline (migrated into, not freshly deployed into) rather
	// than a same-run KMS install.
	// SECURESIGN-5702: the Secret from the file era above still exists, so
	// dropping the KMS settings without naming a replacement used to make the
	// operator adopt it and resume signing with the superseded key.
	It("refuses the revert while the spec names no signer secrets", func(ctx SpecContext) {
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

		const wantRefs = "a signer is generated only on first install"

		Eventually(func() bool {
			return condition.HasFailureForGeneration(
				fulcio.Get(ctx, cli, namespace.Name, s.Name), fulcioActions.CertCondition, wantRefs)
		}).WithTimeout(2*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
			"Fulcio must report why the revert was refused")

		Eventually(func() bool {
			return condition.HasFailureForGeneration(
				tsa.Get(ctx, cli, namespace.Name, s.Name), tsaActions.TSASignerCondition, wantRefs)
		}).WithTimeout(2*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
			"TSA must report why the revert was refused")

		Eventually(func() bool {
			return condition.HasFailureForGeneration(
				rekor.Get(ctx, cli, namespace.Name, s.Name), rekorActions.SignerCondition, wantRefs)
		}).WithTimeout(2*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
			"Rekor must report why the revert was refused")

		Expect(fulcio.Get(ctx, cli, namespace.Name, s.Name).Status.Certificate.PrivateKeyRef).To(BeNil())
		Expect(tsa.Get(ctx, cli, namespace.Name, s.Name).Status.Signer.FileSigner).To(BeNil())
		Expect(rekor.Get(ctx, cli, namespace.Name, s.Name).Status.Signer.KeyRef).To(BeNil())

		retainedFulcio, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraFulcioKeyRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(sha256.Sum256(retainedFulcio)).To(Equal(fileEraFulcioKeyHash))
		retainedTsa, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraTsaKeyRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(sha256.Sum256(retainedTsa)).To(Equal(fileEraTsaKeyHash))
	})

	It("accepts the revert once the customer supplies signer secrets", func(ctx SpecContext) {
		Expect(cli.Create(ctx, fulcio.CreateSecret(namespace.Name, newFulcioSecret))).To(Succeed())
		Expect(cli.Create(ctx, tsa.CreateSecrets(namespace.Name, newTsaSecret))).To(Succeed())
		Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, newRekorSecret))).To(Succeed())

		Eventually(func() error {
			f := securesign.Get(ctx, cli, namespace.Name, s.Name)
			original := f.DeepCopy()

			f.Spec.Fulcio.Signer.File = &rhtasv1.FulcioFile{
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: newFulcioSecret},
					Key:                  "private",
				},
			}
			f.Spec.Fulcio.Signer.CertificateChain.CertificateChainRef = &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: newFulcioSecret},
				Key:                  "cert",
			}

			f.Spec.TimestampAuthority.Signer.CertificateChain = rhtasv1.CertificateChain{
				CertificateChainRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: newTsaSecret},
					Key:                  "certificateChain",
				},
			}
			f.Spec.TimestampAuthority.Signer.File = &rhtasv1.File{
				PrivateKeyRef: rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: newTsaSecret},
					Key:                  "leafPrivateKey",
				},
			}

			f.Spec.Rekor.Signer.KeyRef = &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: newRekorSecret},
				Key:                  "private",
			}

			return cli.Patch(ctx, f, client.MergeFrom(original))
		}).Should(Succeed())
	})

	It("acknowledges trust material drift after reverting to file mode", func(ctx SpecContext) {
		acknowledgeTrustMaterialDrift(ctx, cli, func() apis.ConditionsAwareObject {
			return fulcio.Get(ctx, cli, namespace.Name, s.Name)
		})

		acknowledgeTrustMaterialDrift(ctx, cli, func() apis.ConditionsAwareObject {
			return tsa.Get(ctx, cli, namespace.Name, s.Name)
		})

		acknowledgeTrustMaterialDrift(ctx, cli, func() apis.ConditionsAwareObject {
			return rekor.Get(ctx, cli, namespace.Name, s.Name)
		})
	})

	It("Fulcio returns to Ready on the customer-supplied secret while retaining the original one", func(ctx SpecContext) {
		Eventually(func(ctx context.Context) (bool, error) {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			if f != nil && (f.Spec.Signer.Type == "" || f.Spec.Signer.Type == rhtasv1.SignerTypeFile) &&
				f.Spec.Signer.Kms == nil && condition.IsReadyForGeneration(f) {
				return true, nil
			}
			// Events can outlive a failed rollout; retain them as timeout
			// diagnostics instead of aborting before the controller can recover.
			if msg := failedMountEvent(ctx, cli, namespace.Name, "fulcio-server"); msg != "" {
				return false, fmt.Errorf("Fulcio is not Ready in file mode; recorded FailedMount: %s", msg)
			}
			return false, fmt.Errorf("Fulcio is not Ready in file mode for its current generation")
		}).WithContext(ctx).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
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
			"Fulcio must not reactivate the original file-era private key after the KMS round trip")
		Expect(f.Status.Certificate.PrivateKeyRef.Name).To(Equal(newFulcioSecret),
			"Fulcio must use the secret the customer named, not one of its own")

		privKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.certificate.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(privKey).ToNot(BeEmpty())
		Expect(sha256.Sum256(privKey)).ToNot(Equal(fileEraFulcioKeyHash),
			"Fulcio must sign with the customer's new key, not the superseded file-era one")

		newCert, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(newCert).ToNot(BeEmpty())
		Expect(sha256.Sum256(newCert)).ToNot(Equal(sha256.Sum256(fileEraFulcioCert)),
			"Fulcio must publish the customer's new certificate after the rotation")

		retainedKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraFulcioKeyRef)
		Expect(err).ToNot(HaveOccurred(), "the original Fulcio file signer Secret must be retained")
		Expect(sha256.Sum256(retainedKey)).To(Equal(fileEraFulcioKeyHash),
			"the original Fulcio private key must remain unchanged")
	})

	It("TSA returns to Ready on the customer-supplied secret while retaining the original one", func(ctx SpecContext) {
		Eventually(func(ctx context.Context) (bool, error) {
			t := tsa.Get(ctx, cli, namespace.Name, s.Name)
			if t != nil && (t.Spec.Signer.Type == "" || t.Spec.Signer.Type == rhtasv1.SignerTypeFile) &&
				t.Spec.Signer.Kms == nil && t.Spec.Signer.Tink == nil && condition.IsReadyForGeneration(t) {
				return true, nil
			}
			if msg := failedMountEvent(ctx, cli, namespace.Name, "tsa-server"); msg != "" {
				return false, fmt.Errorf("TSA is not Ready in file mode; recorded FailedMount: %s", msg)
			}
			return false, fmt.Errorf("TSA is not Ready in file mode for its current generation")
		}).WithContext(ctx).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
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
			"TSA must not reactivate the original file-era private key after the KMS round trip")
		Expect(t.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal(newTsaSecret),
			"TSA must use the secret the customer named, not one of its own")
		leafKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.FileSigner.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.signer.fileSigner.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(leafKey).ToNot(BeEmpty())
		Expect(sha256.Sum256(leafKey)).ToNot(Equal(fileEraTsaKeyHash),
			"TSA must sign with the customer's new key, not the superseded file-era one")

		newChain, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(newChain).ToNot(BeEmpty())
		Expect(sha256.Sum256(newChain)).ToNot(Equal(sha256.Sum256(fileEraTsaCert)),
			"TSA must publish the customer's new certificate chain after the rotation")

		retainedKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, fileEraTsaKeyRef)
		Expect(err).ToNot(HaveOccurred(), "the original TSA file signer Secret must be retained")
		Expect(sha256.Sum256(retainedKey)).To(Equal(fileEraTsaKeyHash),
			"the original TSA private key must remain unchanged")
	})

	It("Rekor comes back Ready on the customer-supplied secret while retaining the original one", func(ctx SpecContext) {
		Eventually(func(ctx context.Context) bool {
			r := rekor.Get(ctx, cli, namespace.Name, s.Name)
			if r != nil && (r.Spec.Signer.Type == "" || r.Spec.Signer.Type == rhtasv1.SignerTypeSecret) &&
				r.Spec.Signer.Kms == nil && condition.IsReadyForGeneration(r) {
				return true
			}
			return false
		}).WithContext(ctx).WithTimeout(3*time.Minute).WithPolling(3*time.Second).Should(BeTrue(),
			"Rekor never became Ready after reverting to file mode")

		r := rekor.Get(ctx, cli, namespace.Name, s.Name)
		Expect(r.Status.Signer.KeyRef).ToNot(BeNil())
		Expect(r.Status.Signer.KeyRef.Name).ToNot(Equal(fileEraRekorRef),
			"Rekor must not reuse the original file-era signer secret")
		Expect(r.Status.Signer.KeyRef.Name).To(Equal(newRekorSecret),
			"Rekor must use the secret the customer named, not one of its own")
		Expect(r.Status.Signer.Type).To(Equal(rhtasv1.SignerTypeSecret),
			"the KMS marker must be cleared once the instance is back on a secret signer")

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
			return localCosign.VerifyWithTimestampChain(ctx, targetImageV2, tsChainV2)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

		Eventually(limitMigrationAttempts(func(ctx context.Context) error {
			return localCosign.VerifyWithTimestampChain(ctx, targetImageV1, tsChainV1)
		})).WithContext(ctx).WithTimeout(3 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})
})
