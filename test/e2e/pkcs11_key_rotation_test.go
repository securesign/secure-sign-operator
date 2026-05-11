//go:build integration

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	fulcioActions "github.com/securesign/operator/internal/controller/fulcio/actions"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/test/e2e/support"
	testKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var _ = Describe("PKCS#11 key rotation", Ordered, func() {
	cli, _ := support.CreateClient()
	var (
		targetImageName  string
		namespace        *v1.Namespace
		s                *v1alpha1.Securesign
		runningTimestamp time.Time
		oldCACert        []byte
		tufRepoWorkdir   string
		tufPod           v1.Pod
	)

	BeforeAll(func() {
		if _, err := exec.LookPath("tuftool"); err != nil {
			Skip("tuftool command not found")
		}
		SetDefaultEventuallyTimeout(6 * time.Minute)
	})

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithTSA(),
			securesign.WithPKCS11Certs(),
			securesign.WithManagedDatabase(),
			securesign.WithExternalAccess(),
			securesign.WithDefaultOIDC(),
			securesign.WithNTPMonitoring(),
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with PKCS#11 CA", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
			runningTimestamp = time.Now()
		})

		It("Use cosign cli", func(ctx SpecContext) {
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})

	Describe("Rotate PKCS#11 key via keyConfig.id change", func() {
		It("Download old CA cert", func(ctx SpecContext) {
			f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
			Expect(f).NotTo(BeNil())
			Expect(f.Status.Certificate).NotTo(BeNil())
			Expect(f.Status.Certificate.CARef).NotTo(BeNil())
			var err error
			oldCACert, err = kubernetes.GetSecretData(cli, namespace.Name, f.Status.Certificate.CARef)
			Expect(err).NotTo(HaveOccurred())
			Expect(oldCACert).NotTo(BeEmpty())
		})

		It("Update keyConfig.id to trigger rotation", func(ctx SpecContext) {
			Eventually(func() error {
				f := securesign.Get(ctx, cli, s.Namespace, s.Name)
				f.Spec.Fulcio.Certificate.PKCS11.KeyConfig.ID = 200
				return cli.Update(ctx, f)
			}).Should(Succeed())
		})

		It("Status reflects new keyConfig.id", func(ctx SpecContext) {
			Eventually(func(g Gomega) int {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f).NotTo(BeNil())
				g.Expect(f.Status.Certificate).NotTo(BeNil())
				g.Expect(f.Status.Certificate.PKCS11).NotTo(BeNil())
				return f.Status.Certificate.PKCS11.KeyConfig.ID
			}).Should(Equal(200))
		})

		It("Fulcio pod is recreated after rotation", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				list := &v1.PodList{}
				g.Expect(cli.List(ctx, list, runtimeCli.InNamespace(s.Namespace), runtimeCli.MatchingLabels{labels.LabelAppComponent: fulcioActions.ComponentName})).To(Succeed())
				for _, p := range list.Items {
					if p.CreationTimestamp.After(runningTimestamp) {
						return true
					}
				}
				return false
			}).Should(BeTrue())
		})

		It("PKCS11ConfigAvailable is true", func(ctx SpecContext) {
			Eventually(func(g Gomega) bool {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f).NotTo(BeNil())
				return meta.IsStatusConditionTrue(f.GetConditions(), fulcioActions.PKCS11ConfigCondition)
			}).Should(BeTrue())
		})

		It("New CA cert is different from old cert", func(ctx SpecContext) {
			Eventually(func(g Gomega) {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f).NotTo(BeNil())
				g.Expect(f.Status.Certificate).NotTo(BeNil())
				g.Expect(f.Status.Certificate.CARef).NotTo(BeNil())

				newCACert, err := kubernetes.GetSecretData(cli, namespace.Name, f.Status.Certificate.CARef)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(newCACert).NotTo(BeEmpty())
				g.Expect(newCACert).NotTo(Equal(oldCACert))
			}).Should(Succeed())
		})

		It("Only one secret has the Fulcio discovery label", func(ctx SpecContext) {
			list := &v1.SecretList{}
			Expect(cli.List(ctx, list,
				runtimeCli.InNamespace(namespace.Name),
				runtimeCli.MatchingLabels{labels.LabelNamespace + "/fulcio_v1.crt.pem": "cert"})).To(Succeed())
			Expect(list.Items).To(HaveLen(1))
		})

		It("All components are running after rotation", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})
	})

	Describe("Update TUF repository after rotation", func() {
		var certs string

		It("Download TUF repository", func(ctx SpecContext) {
			var err error
			certs, err = os.MkdirTemp(os.TempDir(), "certs")
			Expect(err).ToNot(HaveOccurred())

			tufRepoWorkdir, err = os.MkdirTemp(os.TempDir(), "tuf-repo")
			Expect(err).ToNot(HaveOccurred())

			tufKeys := &v1.Secret{}
			Expect(os.Mkdir(filepath.Join(tufRepoWorkdir, "keys"), 0777)).To(Succeed())
			Expect(cli.Get(ctx, runtimeCli.ObjectKey{Name: "tuf-root-keys", Namespace: namespace.Name}, tufKeys)).To(Succeed())
			for k, v := range tufKeys.Data {
				Expect(os.WriteFile(filepath.Join(tufRepoWorkdir, "keys", k), v, 0644)).To(Succeed())
			}

			Expect(os.Mkdir(filepath.Join(tufRepoWorkdir, "tuf-repo"), 0777)).To(Succeed())
			tufPodList := &v1.PodList{}
			Expect(cli.List(ctx, tufPodList, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{labels.LabelAppName: tufConstants.DeploymentName})).To(Succeed())
			Expect(tufPodList.Items).To(HaveLen(1))
			tufPod = tufPodList.Items[0]

			Expect(testKubernetes.CopyFromPod(ctx, tufPod, "/var/www/html", filepath.Join(tufRepoWorkdir, "tuf-repo"))).To(Succeed())
		})

		It("Rotate fulcio cert in TUF", func(ctx SpecContext) {
			var newCACert []byte
			Eventually(func(g Gomega) {
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f).NotTo(BeNil())
				g.Expect(f.Status.Certificate).NotTo(BeNil())
				g.Expect(f.Status.Certificate.CARef).NotTo(BeNil())

				var err error
				newCACert, err = kubernetes.GetSecretData(cli, namespace.Name, f.Status.Certificate.CARef)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(newCACert).NotTo(BeEmpty())
			}).Should(Succeed())

			s = securesign.Get(ctx, cli, namespace.Name, s.Name)

			Expect(os.WriteFile(certs+"/fulcio_v1.crt.pem", oldCACert, 0644)).To(Succeed())
			Expect(os.WriteFile(certs+"/new-fulcio.cert.pem", newCACert, 0644)).To(Succeed())

			Expect(clients.ExecuteInDir(certs, "tuftool", tufToolParams("fulcio", "fulcio_v1.crt.pem", s.Status.FulcioStatus.Url, tufRepoWorkdir, true)...)).To(Succeed())
			Expect(clients.ExecuteInDir(certs, "tuftool", tufToolParams("fulcio", "new-fulcio.cert.pem", s.Status.FulcioStatus.Url, tufRepoWorkdir, false)...)).To(Succeed())
		})

		It("Upload the TUF repository back", func(ctx SpecContext) {
			Expect(testKubernetes.CopyToPod(ctx, config.GetConfigOrDie(), tufPod, filepath.Join(tufRepoWorkdir, "tuf-repo"), "/var/www/html")).To(Succeed())
		})
	})

	It("Use cosign cli after rotation", func(ctx SpecContext) {
		newImage := support.PrepareImage(ctx)
		s = securesign.Get(ctx, cli, namespace.Name, s.Name)
		tas.VerifyByCosign(ctx, newImage, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
	})
})
