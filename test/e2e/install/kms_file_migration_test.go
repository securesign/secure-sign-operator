//go:build integration

package install

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// tufToolParams builds the tufcli rhtas flags to set (and optionally expire) a
// single service's TUF target, mirroring lifecycle/key_rotation_test.go so
// both suites drive tufcli identically.
func tufToolParams(component, targetName, url string, workdir string, expire bool) []string {
	args := []string{
		"rhtas",
		"--root", workdir + "/tuf-repo/root.json",
		"--key", workdir + "/keys/snapshot.pem",
		"--key", workdir + "/keys/targets.pem",
		"--key", workdir + "/keys/timestamp.pem",
		fmt.Sprintf("--set-%s-target", component), targetName,
		fmt.Sprintf("--%s-uri", component), url,
		"--outdir", workdir + "/tuf-repo",
		"--metadata-url", "file://" + workdir + "/tuf-repo",
	}
	if expire {
		args = append(args, fmt.Sprintf("--%s-status", component), "Expired")
	}
	return args
}

// failedMountEvent returns the message of the first FailedMount event found for
// any Pod whose name contains podSubstring, or "" if none exists yet. Used to
// fail fast instead of waiting out a full Eventually timeout: FailedMount from
// a bad secret-key reference is a permanent condition, not a transient one, so
// there is nothing to gain from continuing to poll once it appears.
func failedMountEvent(ctx context.Context, cli client.Client, namespace, podSubstring string) string {
	events := &v1.EventList{}
	if err := cli.List(ctx, events, client.InNamespace(namespace)); err != nil {
		return ""
	}
	for _, e := range events.Items {
		if e.Reason == "FailedMount" &&
			e.InvolvedObject.Kind == "Pod" &&
			strings.Contains(e.InvolvedObject.Name, podSubstring) {
			return e.Message
		}
	}
	return ""
}

// Regression test for SECURESIGN-5653: migrating Fulcio/TSA from a KMS signer
// back to a file-based (operator-generated) signer leaves stale
// .status.certificate / .status.signer secret refs pointing at the old KMS
// cert-chain secret, which lacks the private-key data key file mode expects.
// The new pod's volume mount is built from that stale ref and never starts
// (FailedMount: references non-existent secret key). See
// internal/controller/fulcio/actions/generate_signer.go:resolveRef and
// internal/controller/tsa/actions/generate_signer.go:resolveRef.
var _ = Describe("Securesign KMS-to-file signer migration", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace   *v1.Namespace
		s           *rhtasv1.Securesign
		fipsEnabled bool
		oldFulcioCA string
		oldTsaChain string

		targetImageV1                                    string
		targetImageV2                                    string
		oldFulcioCertBytes, oldTsaCertBytes, oldRekorPub []byte
		newFulcioCertBytes, newTsaCertBytes, newRekorPub []byte
		newFulcioCA, newTsaChain                         string
		tufRepoWorkdir                                   string
		tufPod                                           v1.Pod
		localCosign                                      *cosign.LocalCosign
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

	BeforeAll(func(ctx SpecContext) {
		Expect(openbao.CreatePrerequisites(ctx, cli, namespace.Name)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		Expect(openbao.CreateKMSCertificate(ctx, cli, namespace.Name, openbao.FulcioKeyName)).To(Succeed())
		Expect(openbao.CreateKMSTimestampCertificate(ctx, cli, namespace.Name, openbao.TsaKeyName)).To(Succeed())
	})

	BeforeAll(func(ctx SpecContext) {
		// The final KMS phase must use new KMS keys. Reusing the original URIs
		// would only exercise status reconciliation, not KMS key rotation.
		Expect(openbao.CreateTransitKey(ctx, namespace.Name, openbao.RekorKeyNameV2)).To(Succeed())
		Expect(openbao.CreateTransitKey(ctx, namespace.Name, openbao.FulcioKeyNameV2)).To(Succeed())
		Expect(openbao.CreateTransitKey(ctx, namespace.Name, openbao.TsaKeyNameV2)).To(Succeed())
		Expect(openbao.CreateKMSCertificate(ctx, cli, namespace.Name, openbao.FulcioKeyNameV2)).To(Succeed())
		Expect(openbao.CreateKMSTimestampCertificate(ctx, cli, namespace.Name, openbao.TsaKeyNameV2)).To(Succeed())
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
		targetImageV1 = support.PrepareImage(ctx)
	})

	It("all components reach Ready in KMS mode", func(ctx SpecContext) {
		tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
	})

	It("signs and verifies an image while running in KMS mode", func(ctx SpecContext) {
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

	It("captures the KMS-era secret refs", func(ctx SpecContext) {
		f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
		Expect(f).ToNot(BeNil())
		Expect(f.Status.Certificate).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef).ToNot(BeNil())
		oldFulcioCA = f.Status.Certificate.CARef.Name
		var err error
		oldFulcioCertBytes, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(oldFulcioCertBytes).ToNot(BeEmpty())

		t := tsa.Get(ctx, cli, namespace.Name, s.Name)
		Expect(t).ToNot(BeNil())
		Expect(t.Status.Signer).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef).ToNot(BeNil())
		oldTsaChain = t.Status.Signer.CertificateChainRef.Name
		oldTsaCertBytes, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(oldTsaCertBytes).ToNot(BeEmpty())

		// Rekor's KMS-mode reconcile never writes .status.signer.keyRef at all
		// (unlike Fulcio's .status.certificate / TSA's .status.signer, which
		// their respective KMS actions do overwrite) -- confirming there is
		// nothing cached here for a mode switch to mistakenly reuse.
		r := rekor.Get(ctx, cli, namespace.Name, s.Name)
		Expect(r).ToNot(BeNil())
		Expect(r.Status.Signer.KeyRef).To(BeNil(),
			"Rekor should not have a cached signer KeyRef while running in KMS mode")
		Expect(r.Status.PublicKey).ToNot(BeEmpty())
		oldRekorPub = []byte(r.Status.PublicKey)
	})

	It("reverts Fulcio and TSA to operator-generated file signers", func(ctx SpecContext) {
		Eventually(func() error {
			f := securesign.Get(ctx, cli, namespace.Name, s.Name)
			// Patch only the fields we actually change, rather than writing the
			// whole object back. A full Update() round-trips every field this
			// Go client knows about (e.g. spec.ctlog.logs / spec.ctlog.fulcio),
			// which this cluster's installed CRD may not recognize yet — that
			// produces "unknown field" API-server warnings (and potential
			// validation churn) on fields this test never intended to touch.
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

	// Once the new file-based secret mounts successfully, Fulcio/TSA will see
	// their freshly generated cert differs from what's cached in
	// .status.certificateChain and sit in TrustMaterialAvailable=Drifted until
	// acknowledged — the same drift gate exercised for any other key change.
	// This is expected, unrelated to the FailedMount bug, and required before
	// either component can reach Ready.
	It("acknowledges trust material drift on the reverted components", func(ctx SpecContext) {
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

	It("Fulcio comes back Ready with a freshly generated file secret", func(ctx SpecContext) {
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
		Expect(f.Status.Certificate.CARef.Name).ToNot(Equal(oldFulcioCA),
			"Fulcio status still references the old KMS cert-chain secret after reverting to file mode")
		newFulcioCA = f.Status.Certificate.CARef.Name

		privKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.certificate.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(privKey).ToNot(BeEmpty())

		newFulcioCertBytes, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(newFulcioCertBytes).ToNot(BeEmpty())
	})

	It("TSA comes back Ready with a freshly generated file secret", func(ctx SpecContext) {
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
		Expect(t.Status.Signer.CertificateChainRef.Name).ToNot(Equal(oldTsaChain),
			"TSA status still references the old KMS cert-chain secret after reverting to file mode")
		newTsaChain = t.Status.Signer.CertificateChainRef.Name

		Expect(t.Status.Signer.FileSigner).ToNot(BeNil())
		Expect(t.Status.Signer.FileSigner.PrivateKeyRef).ToNot(BeNil())
		leafKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.FileSigner.PrivateKeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.signer.fileSigner.privateKeyRef must resolve to a real secret key, not a stale KMS ref")
		Expect(leafKey).ToNot(BeEmpty())

		newTsaCertBytes, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(newTsaCertBytes).ToNot(BeEmpty())
	})

	// Rekor was never expected to hit the stale-ref bug (see the KMS-mode
	// baseline check above), but this closes the loop: after reverting, it
	// must resolve to a fresh, real signer secret via the normal
	// generateSigner path, not silently keep using whatever key it had in
	// KMS mode (which was never file-backed to begin with).
	It("Rekor comes back Ready with a freshly generated file secret", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(rekor.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Rekor never became Ready after reverting to file mode")

		r := rekor.Get(ctx, cli, namespace.Name, s.Name)
		Expect(r.Status.Signer.KeyRef).ToNot(BeNil())

		privKey, err := kubernetes.GetSecretData(ctx, cli, namespace.Name, r.Status.Signer.KeyRef)
		Expect(err).ToNot(HaveOccurred(),
			"status.signer.keyRef must resolve to a real secret key")
		Expect(privKey).ToNot(BeEmpty())

		Expect(r.Status.PublicKey).ToNot(BeEmpty())
		newRekorPub = []byte(r.Status.PublicKey)
	})

	It("all components remain Ready end-to-end after the migration", func(ctx SpecContext) {
		s = securesign.Get(ctx, cli, namespace.Name, s.Name)
		tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
	})

	// TUF is not kept in sync automatically -- it only reflects whatever was
	// captured at tuf-repository-init time, so the manual tufcli dance below
	// is genuinely required, not a workaround. Mirrors
	// lifecycle/key_rotation_test.go's "Update TUF repository" flow.
	Describe("Update TUF repository", func() {
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
			Expect(os.WriteFile(certs+"/fulcio_v1.crt.pem", oldFulcioCertBytes, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-fulcio.cert.pem", newFulcioCertBytes, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "fulcio_v1.crt.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "new-fulcio.cert.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the rekor target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/rekor.pub", oldRekorPub, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-rekor.pub", newRekorPub, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "rekor.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "new-rekor.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the tsa target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/tsa.certchain.pem", oldTsaCertBytes, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-tsa.certchain.pem", newTsaCertBytes, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "tsa.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "new-tsa.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("uploads the TUF repository back", func(ctx SpecContext) {
			Expect(testKubernetes.CopyToPod(ctx, config.GetConfigOrDie(), tufPod, filepath.Join(tufRepoWorkdir, "tuf-repo"), "/var/www/html")).To(Succeed())
		})
	})

	It("signs and verifies a new image, and confirms the KMS-era image still verifies (backward compatibility)", func(ctx SpecContext) {
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

		// The critical assertion: the image signed back when Fulcio/Rekor/TSA
		// were still in KMS mode must still verify now that they've been
		// reverted to file-based signers with entirely new keys/certs. TUF's
		// expired-but-retained KMS-era entries are what make this possible.
		Eventually(func(ctx context.Context) error {
			return localCosign.Verify(ctx, targetImageV1)
		}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
	})

	// Belt-and-suspenders round trip back to KMS: File -> KMS is not expected
	// to hit SECURESIGN-5653 (see file_kms_migration_test.go), but exercising
	// it here too guards against a future change to resolve_kms_signer.go /
	// resolve_kms_tink_signer.go silently breaking that assumption.
	It("migrates Fulcio, TSA and Rekor back to KMS mode", func(ctx SpecContext) {
		Eventually(func() error {
			f := securesign.Get(ctx, cli, namespace.Name, s.Name)
			original := f.DeepCopy()

			securesign.WithKMSOpenBaoSignerKey(namespace.Name, openbao.RekorKeyNameV2)(f)
			securesign.WithKMSOpenBaoFulcioSignerKey(namespace.Name, openbao.FulcioKeyNameV2)(f)
			securesign.WithKMSOpenBaoTSASignerKey(namespace.Name, openbao.TsaKeyNameV2)(f)

			return cli.Patch(ctx, f, client.MergeFrom(original))
		}).Should(Succeed())
	})

	It("acknowledges trust material drift after migrating back to KMS mode", func(ctx SpecContext) {
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

	var (
		secondKmsFulcioCert []byte
		secondKmsTsaCert    []byte
		secondKmsRekorPub   []byte
	)

	It("Fulcio comes back Ready with the KMS-backed cert again", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(fulcio.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Fulcio never became Ready after migrating back to KMS mode")

		f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
		Expect(f.Status.Certificate).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef).ToNot(BeNil())
		Expect(f.Status.Certificate.CARef.Name).ToNot(Equal(newFulcioCA))
		Expect(f.Status.Certificate.CARef.Name).To(Equal(openbao.CertChainSecretName(openbao.FulcioKeyNameV2)))
		var err error
		secondKmsFulcioCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, f.Status.Certificate.CARef)
		Expect(err).ToNot(HaveOccurred())
		Expect(secondKmsFulcioCert).ToNot(BeEmpty())
	})

	It("TSA comes back Ready with the KMS-backed cert again", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(tsa.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"TimestampAuthority never became Ready after migrating back to KMS mode")

		t := tsa.Get(ctx, cli, namespace.Name, s.Name)
		Expect(t.Status.Signer).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef).ToNot(BeNil())
		Expect(t.Status.Signer.CertificateChainRef.Name).ToNot(Equal(newTsaChain))
		Expect(t.Status.Signer.CertificateChainRef.Name).To(Equal(openbao.CertChainSecretName(openbao.TsaKeyNameV2)))
		var err error
		secondKmsTsaCert, err = kubernetes.GetSecretData(ctx, cli, namespace.Name, t.Status.Signer.CertificateChainRef)
		Expect(err).ToNot(HaveOccurred())
		Expect(secondKmsTsaCert).ToNot(BeEmpty())
	})

	It("Rekor comes back Ready with the KMS-backed key again", func(ctx SpecContext) {
		Eventually(func() bool {
			return condition.IsReady(rekor.Get(ctx, cli, namespace.Name, s.Name))
		}).WithTimeout(60*time.Second).WithPolling(3*time.Second).Should(BeTrue(),
			"Rekor never became Ready after migrating back to KMS mode")

		r := rekor.Get(ctx, cli, namespace.Name, s.Name)
		Expect(r.Status.PublicKey).ToNot(BeEmpty())
		Expect(r.Status.PublicKey).ToNot(Equal(string(oldRekorPub)))
		Expect(r.Status.PublicKey).ToNot(Equal(string(newRekorPub)))
		secondKmsRekorPub = []byte(r.Status.PublicKey)
	})

	It("all components remain Ready end-to-end after migrating back to KMS mode", func(ctx SpecContext) {
		s = securesign.Get(ctx, cli, namespace.Name, s.Name)
		tas.VerifyAllComponents(ctx, cli, s, !fipsEnabled, true)
	})

	Describe("Sync TUF after migrating back to KMS mode", func() {
		var certs string

		BeforeAll(func() {
			var err error
			certs, err = os.MkdirTemp(os.TempDir(), "certs")
			Expect(err).ToNot(HaveOccurred())
		})

		It("syncs the fulcio target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/new-fulcio.cert.pem", newFulcioCertBytes, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-fulcio-kms2.cert.pem", secondKmsFulcioCert, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "new-fulcio.cert.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("fulcio", "new-fulcio-kms2.cert.pem", s.Status.FulcioStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the rekor target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/new-rekor.pub", newRekorPub, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-rekor-kms2.pub", secondKmsRekorPub, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "new-rekor.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("rekor", "new-rekor-kms2.pub", s.Status.RekorStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("syncs the tsa target", func(ctx SpecContext) {
			Expect(os.WriteFile(certs+"/new-tsa.certchain.pem", newTsaCertBytes, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-tsa-kms2.certchain.pem", secondKmsTsaCert, 0644)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "new-tsa.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tufcli", tufToolParams("tsa", "new-tsa-kms2.certchain.pem", s.Status.TSAStatus.URL, tufRepoWorkdir, false)...)).To(Succeed())
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
