//go:build integration

package install

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
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

// Companion to kms_file_migration_test.go: a real file-mode install migrated
// to KMS, then back to file. File -> KMS is not expected to hit
// SECURESIGN-5653 -- entering KMS mode never reads from .status at all, only
// from spec (resolve_kms_signer.go / resolve_kms_tink_signer.go require
// CertificateChainRef to already be set on the incoming spec, which CEL
// validation enforces). The round trip back to file is kept anyway as a
// belt-and-suspenders guard: today it's provably the same code path as
// kms_file_migration_test.go's revert (resolve_kms_signer.go /
// resolve_kms_tink_signer.go fully overwrite the relevant status fields
// rather than merging, so KMS-mode status doesn't depend on how KMS mode was
// reached), but that equivalence could silently break under a future change
// to those actions, and a round trip catches that where a one-way test can't.
var _ = Describe("Securesign file-to-KMS-to-file signer migration", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace   *v1.Namespace
		s           *rhtasv1.Securesign
		fipsEnabled bool

		targetImageV1 string
		targetImageV2 string
		localCosign   *cosign.LocalCosign

		fileEraFulcioRef  string
		fileEraTsaRef     string
		fileEraRekorRef   string
		fileEraFulcioCert []byte
		fileEraTsaCert    []byte
		fileEraRekorPub   []byte

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

		Eventually(func(ctx context.Context) error {
			return clients.Execute("cosign", "initialize", "--mirror="+s.Status.TufStatus.URL, "--root="+s.Status.TufStatus.URL+"/root.json")
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(func(ctx context.Context) error {
			return localCosign.Sign(ctx, targetImageV1)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV1)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
	})

	It("captures the file-era secret refs", func(ctx SpecContext) {
		f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
		Expect(f.Status.Certificate).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef).ToNot(BeNil())
		fileEraFulcioRef = f.Status.Certificate.CARef.Name
		var err error
		fileEraFulcioCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(fileEraFulcioCert).ToNot(BeEmpty())

		t := tsa.Get(ctx, cli, namespace.Name, s.Name)
		Expect(t.Status.Signer).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef).ToNot(BeNil())
		fileEraTsaRef = t.Status.Signer.CertificateChainRef.Name
		fileEraTsaCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(fileEraTsaCert).ToNot(BeEmpty())

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
		Eventually(func(ctx context.Context) error {
			return clients.Execute("cosign", "initialize", "--mirror="+s.Status.TufStatus.URL, "--root="+s.Status.TufStatus.URL+"/root.json")
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())

		targetImageV2 = support.PrepareImage(ctx)
		Eventually(func(ctx context.Context) error {
			return localCosign.Sign(ctx, targetImageV2)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV2)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())

		Eventually(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV1)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
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

	It("Fulcio comes back Ready with a freshly generated file secret (SECURESIGN-5653 regression)", func(ctx SpecContext) {
		Eventually(func() (bool, error) {
			if msg := failedMountEvent(ctx, cli, namespace.Name, "fulcio-server"); msg != "" {
				return false, StopTrying(fmt.Sprintf(
					"Fulcio pod hit FailedMount — stale KMS secret ref not cleared on mode switch "+
						"(internal/controller/fulcio/actions/generate_signer.go resolveRef): %s", msg))
			}
			return condition.IsReady(fulcio.Get(ctx, cli, namespace.Name, s.Name)), nil
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Fulcio never became Ready after reverting to file mode")

		f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
		Expect(f.Status.Certificate).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef.Name).ToNot(Equal(kmsEraFulcioRef),
			"Fulcio status still references the old KMS cert-chain secret after reverting to file mode")

		privKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.certificate.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(privKey).ToNot(BeEmpty())
	})

	It("TSA comes back Ready with a freshly generated file secret (SECURESIGN-5653 regression)", func(ctx SpecContext) {
		Eventually(func() (bool, error) {
			if msg := failedMountEvent(ctx, cli, namespace.Name, "tsa-server"); msg != "" {
				return false, StopTrying(fmt.Sprintf(
					"TSA pod hit FailedMount — stale KMS secret ref not cleared on mode switch "+
						"(internal/controller/tsa/actions/generate_signer.go resolveRef): %s", msg))
			}
			return condition.IsReady(tsa.Get(ctx, cli, namespace.Name, s.Name)), nil
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"TimestampAuthority never became Ready after reverting to file mode")

		t := tsa.Get(ctx, cli, namespace.Name, s.Name)
		Expect(t.Status.Signer).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef.Name).ToNot(Equal(kmsEraTsaRef),
			"TSA status still references the old KMS cert-chain secret after reverting to file mode")

		Expect(t.Status.Signer.FileSigner).ToNot(BeNil())
		Expect(t.Status.Signer.FileSigner.PrivateKeyRef).ToNot(BeNil())
		leafKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.FileSigner.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.signer.fileSigner.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(leafKey).ToNot(BeEmpty())
	})

	It("Rekor comes back Ready with a freshly generated file secret", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(rekor.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
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
		Expect(oldPrivKey).ToNot(Equal(privKey), "file signer rotation must create a new key pair")
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
		Eventually(func(ctx context.Context) error {
			return clients.Execute("cosign", "initialize", "--mirror="+s.Status.TufStatus.URL, "--root="+s.Status.TufStatus.URL+"/root.json")
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())

		targetImageV3 := support.PrepareImage(ctx)
		Eventually(func(ctx context.Context) error {
			return localCosign.Sign(ctx, targetImageV3)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
		Eventually(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV3)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())

		Eventually(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV2)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())

		Eventually(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV1)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
	})
})
