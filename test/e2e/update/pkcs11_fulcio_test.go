//go:build integration

package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/securesign/operator/internal/constants"
	fulcioAction "github.com/securesign/operator/internal/controller/fulcio/actions"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	testKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

var _ = Describe("PKCS#11 Fulcio update", Ordered, func() {
	SetDefaultEventuallyTimeout(time.Duration(8) * time.Minute)
	cli, _ := support.CreateClient()

	var (
		namespace      *v1.Namespace
		s              *v1alpha1.Securesign
		oldCACert      []byte
		tufRepoWorkdir string
		tufPod         v1.Pod
	)

	BeforeAll(func() {
		if _, err := exec.LookPath("tuftool"); err != nil {
			Skip("tuftool command not found")
		}
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
			securesign.WithoutSearchUI(),
		)
	})

	Describe("Install with PKCS#11 certificates", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})
	})

	Describe("Changes OIDC configuration with PKCS#11 baseline", func() {
		var fulcioGeneration int64

		It("stored current deployment observed generations", func(ctx SpecContext) {
			fulcioGeneration = getDeploymentGeneration(ctx, cli,
				types.NamespacedName{Namespace: namespace.Name, Name: fulcioAction.DeploymentName},
			)
			Expect(fulcioGeneration).Should(BeNumerically(">", 0))
		})

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

		It("adds new OIDCIssuers", func(ctx SpecContext) {
			Eventually(func(g Gomega) error {
				g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(s), s)).To(Succeed())
				s.Spec.Fulcio.Config.OIDCIssuers = append(s.Spec.Fulcio.Config.OIDCIssuers, v1alpha1.OIDCIssuer{
					ClientID:  "fake",
					IssuerURL: "fake",
					Issuer:    "fake",
					Type:      "email",
				})
				return cli.Update(ctx, s)
			}).WithTimeout(1 * time.Second).Should(Succeed())
		})

		It("has status Ready", func(ctx SpecContext) {
			Eventually(func(g Gomega) string {
				ctl := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(ctl).NotTo(BeNil())
				return meta.FindStatusCondition(ctl.Status.Conditions, constants.ReadyCondition).Reason
			}).Should(Equal(state.Ready.String()))
		})

		It("updated Fulcio deployment", func(ctx SpecContext) {
			Eventually(func() int64 {
				return getDeploymentGeneration(ctx, cli, types.NamespacedName{Namespace: namespace.Name, Name: fulcioAction.DeploymentName})
			}).Should(BeNumerically(">", fulcioGeneration))
		})

		It("verify Fulcio is ready", func(ctx SpecContext) {
			fulcio.Verify(ctx, cli, namespace.Name, s.Name)
		})

		It("verify new OIDC configuration in ConfigMap", func(ctx SpecContext) {
			var f *v1alpha1.Fulcio
			var fulcioPod *v1.Pod
			Eventually(func(g Gomega) {
				f = fulcio.Get(ctx, cli, namespace.Name, s.Name)
				g.Expect(f).NotTo(BeNil())
				fulcioPod = fulcio.GetServerPod(ctx, cli, namespace.Name)()
				g.Expect(fulcioPod).NotTo(BeNil())
			}).Should(Succeed())

			var configVolName string
			for _, vol := range fulcioPod.Spec.Volumes {
				if vol.ConfigMap != nil && vol.ConfigMap.Name == f.Status.ServerConfigRef.Name {
					configVolName = vol.Name
					break
				}
			}
			Expect(configVolName).NotTo(BeEmpty(), "fulcio-config volume should reference the server config ConfigMap")

			cm := &v1.ConfigMap{}
			Expect(cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: f.Status.ServerConfigRef.Name}, cm)).To(Succeed())
			config := &fulcioAction.FulcioMapConfig{}
			Expect(yaml.Unmarshal([]byte(cm.Data["config.yaml"]), config)).To(Succeed())
			Expect(config.OIDCIssuers).To(HaveKey("fake"))
		})

		It("Fulcio still uses PKCS#11 CA args", func(ctx SpecContext) {
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			var fulcioContainer *v1.Container
			for i := range server.Spec.Containers {
				if server.Spec.Containers[i].Name == "fulcio-server" {
					fulcioContainer = &server.Spec.Containers[i]
					break
				}
			}
			Expect(fulcioContainer).NotTo(BeNil())
			Expect(fulcioContainer.Args).To(ContainElements("--ca=pkcs11ca"))
			Expect(fulcioContainer.Args).NotTo(ContainElements("--ca=fileca"))
		})

		It("all components are ready after OIDC update", func(ctx SpecContext) {
			tas.VerifyAllComponents(ctx, cli, s, true)
		})

		Describe("Update TUF repository after OIDC-triggered cert change", func() {
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
				f := fulcio.Get(ctx, cli, namespace.Name, s.Name)
				Expect(f).NotTo(BeNil())
				Expect(f.Status.Certificate).NotTo(BeNil())
				Expect(f.Status.Certificate.CARef).NotTo(BeNil())

				newCACert, err := kubernetes.GetSecretData(cli, namespace.Name, f.Status.Certificate.CARef)
				Expect(err).NotTo(HaveOccurred())
				Expect(newCACert).NotTo(BeEmpty())

				s = securesign.Get(ctx, cli, namespace.Name, s.Name)

				Expect(os.WriteFile(certs+"/fulcio_v1.crt.pem", oldCACert, 0644)).To(Succeed())
				Expect(os.WriteFile(certs+"/new-fulcio.cert.pem", newCACert, 0644)).To(Succeed())

				Expect(clients.ExecuteInDir(certs, "tuftool", pkcs11UpdateTufToolParams("fulcio", "fulcio_v1.crt.pem", s.Status.FulcioStatus.Url, tufRepoWorkdir, true)...)).To(Succeed())
				Expect(clients.ExecuteInDir(certs, "tuftool", pkcs11UpdateTufToolParams("fulcio", "new-fulcio.cert.pem", s.Status.FulcioStatus.Url, tufRepoWorkdir, false)...)).To(Succeed())
			})

			It("Upload the TUF repository back", func(ctx SpecContext) {
				Expect(testKubernetes.CopyToPod(ctx, config.GetConfigOrDie(), tufPod, filepath.Join(tufRepoWorkdir, "tuf-repo"), "/var/www/html")).To(Succeed())
			})
		})

		It("verify by cosign", NodeTimeout(10*time.Minute), func(ctx SpecContext) {
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
			tuf.Verify(ctx, cli, namespace.Name, s.Name)
			newImage := support.PrepareImage(ctx)
			s = securesign.Get(ctx, cli, namespace.Name, s.Name)
			tas.VerifyByCosign(ctx, newImage, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
		})
	})
})

func pkcs11UpdateTufToolParams(component, targetName, url string, workdir string, expire bool) []string {
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
