//go:build upgrade

package e2e

import (
	"context"
	"os"
	"strings"
	"time"

	semver "github.com/blang/semver/v4"
	"github.com/google/uuid"
	v12 "github.com/operator-framework/api/pkg/operators/v1"
	tasv1alpha "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils"
	"github.com/securesign/operator/controllers/constants"
	ctl "github.com/securesign/operator/controllers/ctlog/actions"
	fulcio "github.com/securesign/operator/controllers/fulcio/actions"
	rekor "github.com/securesign/operator/controllers/rekor/actions"
	trillian "github.com/securesign/operator/controllers/trillian/actions"
	tuf "github.com/securesign/operator/controllers/tuf/actions"
	"github.com/securesign/operator/e2e/support/tas"
	clients "github.com/securesign/operator/e2e/support/tas/cli"
	v13 "k8s.io/api/apps/v1"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/securesign/operator/e2e/support"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

const testCatalog = "test-catalog"

var _ = Describe("Operator upgrade", Ordered, func() {
	targetImageName := "ttl.sh/" + uuid.New().String() + ":15m"
	cli, _ := CreateClient()
	ctx := context.TODO()

	var (
		namespace                              *v1.Namespace
		baseCatalogImage, targetedCatalogImage string
		base, updated                          semver.Version
		securesignDeployment                   *tasv1alpha.Securesign
		rfulcio                                *tasv1alpha.Fulcio
		rrekor                                 *tasv1alpha.Rekor
		rtuf                                   *tasv1alpha.Tuf
		oidcToken                              string
	)

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			if val, present := os.LookupEnv("CI"); present && val == "true" {
				if val, present := os.LookupEnv("CI"); present && val == "true" {
					support.DumpNamespace(ctx, cli, namespace.Name)
				}
			}
		}
	})

	BeforeAll(func() {

		baseCatalogImage = os.Getenv("TEST_BASE_CATALOG")
		targetedCatalogImage = os.Getenv("TEST_TARGET_CATALOG")

		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			cli.Delete(ctx, namespace)
		})
	})

	BeforeAll(func() {
		support.PrepareImage(ctx, targetImageName)
	})

	It("Install catalogSource", func() {
		gomega.Expect(baseCatalogImage).To(gomega.Not(gomega.BeEmpty()))
		gomega.Expect(targetedCatalogImage).To(gomega.Not(gomega.BeEmpty()))

		gomega.Expect(support.CreateOrUpdateCatalogSource(ctx, cli, namespace.Name, testCatalog, baseCatalogImage)).To(gomega.Succeed())

		gomega.Eventually(func() *v1alpha1.CatalogSource {
			c := &v1alpha1.CatalogSource{}
			cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: testCatalog}, c)
			return c
		}).Should(gomega.And(gomega.Not(gomega.BeNil()), gomega.WithTransform(func(c *v1alpha1.CatalogSource) string {
			if c.Status.GRPCConnectionState == nil {
				return ""
			}
			return c.Status.GRPCConnectionState.LastObservedState
		}, gomega.Equal("READY"))))
	})

	It("Install TAS", func() {
		og := &v12.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "e2e-test",
			},
			Spec: v12.OperatorGroupSpec{
				TargetNamespaces: []string{},
			},
		}
		gomega.Expect(cli.Create(ctx, og)).To(gomega.Succeed())
		subscription := &v1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test",
				Namespace: namespace.Name,
			},
			Spec: &v1alpha1.SubscriptionSpec{
				CatalogSource:          testCatalog,
				CatalogSourceNamespace: namespace.Name,
				Package:                "rhtas-operator",
				Channel:                "stable",
			},
			Status: v1alpha1.SubscriptionStatus{},
		}

		gomega.Expect(cli.Create(ctx, subscription)).To(gomega.Succeed())

		gomega.Eventually(func() {
			csvs := &v1alpha1.ClusterServiceVersionList{}
			gomega.Expect(cli.List(ctx, csvs, runtimeCli.InNamespace(namespace.Name))).To(gomega.Succeed())
		})

		gomega.Eventually(findClusterServiceVersion(ctx, cli, func(_ v1alpha1.ClusterServiceVersion) bool {
			return true
		}, namespace.Name)).Should(gomega.Not(gomega.BeNil()))

		base = findClusterServiceVersion(ctx, cli, func(_ v1alpha1.ClusterServiceVersion) bool {
			return true
		}, namespace.Name)().Spec.Version.Version

		gomega.Eventually(func() []v13.Deployment {
			list := &v13.DeploymentList{}
			gomega.Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{"app.kubernetes.io/part-of": "rhtas-operator"})).To(gomega.Succeed())
			return list.Items
		}).Should(gomega.And(gomega.HaveLen(1), gomega.WithTransform(func(items []v13.Deployment) int32 {
			return items[0].Status.AvailableReplicas
		}, gomega.BeNumerically(">=", 1))))
	})

	It("Install securesign", func() {
		securesignDeployment = &tasv1alpha.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "test",
				Annotations: map[string]string{
					"rhtas.redhat.com/metrics": "false",
				},
			},
			Spec: tasv1alpha.SecuresignSpec{
				Rekor: tasv1alpha.RekorSpec{
					ExternalAccess: tasv1alpha.ExternalAccess{
						Enabled: true,
					},
					RekorSearchUI: tasv1alpha.RekorSearchUI{
						Enabled: utils.Pointer(true),
					},
				},
				Fulcio: tasv1alpha.FulcioSpec{
					ExternalAccess: tasv1alpha.ExternalAccess{
						Enabled: true,
					},
					Config: tasv1alpha.FulcioConfig{
						OIDCIssuers: []tasv1alpha.OIDCIssuer{
							{
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Issuer:    support.OidcIssuerUrl(),
								Type:      "email",
							},
						}},
					Certificate: tasv1alpha.FulcioCert{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "fulcio",
					},
				},
				Ctlog: tasv1alpha.CTlogSpec{},
				Tuf: tasv1alpha.TufSpec{
					ExternalAccess: tasv1alpha.ExternalAccess{
						Enabled: true,
					},
				},
				Trillian: tasv1alpha.TrillianSpec{Db: tasv1alpha.TrillianDB{
					Create: utils.Pointer(true),
				}},
			},
		}

		gomega.Expect(cli.Create(ctx, securesignDeployment)).To(gomega.Succeed())

		tas.VerifySecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyFulcio(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekor(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTrillian(ctx, cli, namespace.Name, securesignDeployment.Name, true)
		tas.VerifyCTLog(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTuf(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekorSearchUI(ctx, cli, namespace.Name, securesignDeployment.Name)
	})

	It("Sign image with cosign cli", func() {
		rfulcio = tas.GetFulcio(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(rfulcio).ToNot(gomega.BeNil())

		rrekor = tas.GetRekor(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(rrekor).ToNot(gomega.BeNil())

		rtuf = tas.GetTuf(ctx, cli, namespace.Name, securesignDeployment.Name)()
		gomega.Expect(rtuf).ToNot(gomega.BeNil())

		var err error
		oidcToken, err = support.OidcToken(ctx)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
		gomega.Expect(oidcToken).ToNot(gomega.BeEmpty())

		// sleep for a while to be sure everything has settled down
		time.Sleep(time.Duration(10) * time.Second)

		gomega.Expect(clients.Execute("cosign", "initialize", "--mirror="+rtuf.Status.Url, "--root="+rtuf.Status.Url+"/root.json")).To(gomega.Succeed())

		gomega.Expect(clients.Execute(
			"cosign", "sign", "-y",
			"--fulcio-url="+rfulcio.Status.Url,
			"--rekor-url="+rrekor.Status.Url,
			"--oidc-issuer="+support.OidcIssuerUrl(),
			"--oidc-client-id="+support.OidcClientID(),
			"--identity-token="+oidcToken,
			targetImageName,
		)).To(gomega.Succeed())
	})

	It("Upgrade operator", func() {
		gomega.Expect(support.CreateOrUpdateCatalogSource(ctx, cli, namespace.Name, testCatalog, targetedCatalogImage)).To(gomega.Succeed())

		time.Sleep(5 * time.Second)
		gomega.Eventually(func() *v1alpha1.CatalogSource {
			c := &v1alpha1.CatalogSource{}
			cli.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: testCatalog}, c)
			return c
		}).Should(gomega.And(gomega.Not(gomega.BeNil()), gomega.WithTransform(func(c *v1alpha1.CatalogSource) string {
			if c.Status.GRPCConnectionState == nil {
				return ""
			}
			return c.Status.GRPCConnectionState.LastObservedState
		}, gomega.Equal("READY"))))

		gomega.Eventually(findClusterServiceVersion(ctx, cli, func(csv v1alpha1.ClusterServiceVersion) bool {
			return csv.Spec.Version.Version.String() != base.String()
		}, namespace.Name)).Should(gomega.Not(gomega.BeNil()))

		updated = findClusterServiceVersion(ctx, cli, func(csv v1alpha1.ClusterServiceVersion) bool {
			return csv.Spec.Version.Version.String() != base.String()
		}, namespace.Name)().Spec.Version.Version
	})

	It("Verify deployment was upgraded", func() {
		gomega.Expect(updated.GT(base)).To(gomega.BeTrue())

		for k, v := range map[string]string{
			fulcio.DeploymentName:            constants.FulcioServerImage,
			ctl.DeploymentName:               constants.CTLogImage,
			tuf.DeploymentName:               constants.TufImage,
			rekor.ServerDeploymentName:       constants.RekorServerImage,
			rekor.SearchUiDeploymentName:     constants.RekorSearchUiImage,
			trillian.LogsignerDeploymentName: constants.TrillianLogSignerImage,
			trillian.LogserverDeploymentName: constants.TrillianServerImage,
		} {
			gomega.Eventually(func() string {
				d := &v13.Deployment{}
				gomega.Expect(cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      k,
				}, d)).To(gomega.Succeed())

				return d.Spec.Template.Spec.Containers[0].Image
			}).Should(gomega.Equal(v))
		}

		tas.VerifySecuresign(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyFulcio(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekor(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTrillian(ctx, cli, namespace.Name, securesignDeployment.Name, true)
		tas.VerifyCTLog(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyTuf(ctx, cli, namespace.Name, securesignDeployment.Name)
		tas.VerifyRekorSearchUI(ctx, cli, namespace.Name, securesignDeployment.Name)
	})

	It("Verify image signature after upgrade", func() {
		gomega.Expect(clients.Execute(
			"cosign", "verify",
			"--rekor-url="+rrekor.Status.Url,
			"--certificate-identity-regexp", ".*@redhat",
			"--certificate-oidc-issuer-regexp", ".*keycloak.*",
			targetImageName,
		)).To(gomega.Succeed())
	})
})

func findClusterServiceVersion(ctx context.Context, cli runtimeCli.Client, conditions func(version v1alpha1.ClusterServiceVersion) bool, ns string) func() *v1alpha1.ClusterServiceVersion {
	return func() *v1alpha1.ClusterServiceVersion {
		lst := v1alpha1.ClusterServiceVersionList{}
		if err := cli.List(ctx, &lst, runtimeCli.InNamespace(ns)); err != nil {
			panic(err)
		}
		for _, s := range lst.Items {
			if strings.Contains(s.Name, "rhtas-operator") && conditions(s) {
				return &s
			}
		}
		return nil
	}
}
