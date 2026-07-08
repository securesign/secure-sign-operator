//go:build upgrade

package e2e

import (
	"fmt"
	"os"
	"time"

	"github.com/blang/semver/v4"
	"github.com/onsi/ginkgo/v2/dsl/core"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/constants"
	ctl "github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcioAction "github.com/securesign/operator/internal/controller/fulcio/actions"
	rekorAction "github.com/securesign/operator/internal/controller/rekor/actions"
	trillianAction "github.com/securesign/operator/internal/controller/trillian/actions"
	tsaAction "github.com/securesign/operator/internal/controller/tsa/actions"
	tufAction "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/images"
	"github.com/securesign/operator/internal/labels"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/kubernetes/olm"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v13 "k8s.io/api/apps/v1"
	rbacV1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support"
	cosignSupport "github.com/securesign/operator/test/e2e/support/tas/cosign"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Operator upgrade", Ordered, func() {
	gomega.SetDefaultEventuallyTimeout(5 * time.Minute)
	cli, _ := support.CreateClient()

	var (
		namespace                              *v1.Namespace
		baseCatalogImage, targetedCatalogImage string
		baseVersion                            string
		securesignDeployment                   *v1alpha1.Securesign
		prevImageName, newImageName            string
		err                                    error
		extension                              olm.Extension
		catalog                                olm.ExtensionSource
		cosign                                 cosignSupport.Cosign
		fipsEnabled                            bool
	)

	BeforeAll(steps.DetectAndConfigureFIPS(cli, func(enabled bool) {
		fipsEnabled = enabled
	}))

	BeforeAll(steps.CreateNamespaceWithoutPSA(cli, func(new *v1.Namespace) {
		namespace = new
	}))
	BeforeAll(func(ctx SpecContext) {
		DeferCleanup(func(ctx SpecContext) {
			if extension != nil {
				_ = cli.Delete(ctx, extension.Unwrap())
			}
			if catalog != nil {
				_ = cli.Delete(ctx, catalog.Unwrap())
			}
			if _, ok := os.LookupEnv("OLM_V1"); ok {
				_ = cli.Delete(ctx, &rbacV1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%s-installer-%s", "rhtas-operator", namespace.Name),
					},
				})
			}
		})
	})

	JustAfterEach(func(ctx SpecContext) {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			core.GinkgoWriter.Println("----------------------- Dumping operator resources -----------------------")
			core.GinkgoWriter.Println("\n\nCatalog:")
			core.GinkgoWriter.Printf("%s ready: %v\n", catalog.GetName(), catalog.IsReady(ctx, cli))
			core.GinkgoWriter.Println("\n\nExtension:")
			core.GinkgoWriter.Printf("%s version: %s ready: %v\n", extension.GetName(), extension.GetVersion(ctx, cli), extension.IsReady(ctx, cli))
		}
	})

	BeforeAll(func(ctx SpecContext) {
		baseCatalogImage = os.Getenv("TEST_BASE_CATALOG")
		targetedCatalogImage = os.Getenv("TEST_TARGET_CATALOG")
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	BeforeAll(func(ctx SpecContext) {
		prevImageName = support.PrepareImage(ctx)
		newImageName = support.PrepareImage(ctx)
	})

	It("Install TAS", func(ctx SpecContext) {
		channel := support.EnvOrDefault("TEST_UPGRADE_CHANNEL", "stable")
		if _, ok := os.LookupEnv("OLM_V1"); ok {
			extension, catalog, err = olm.OlmV1Installer(ctx, cli, baseCatalogImage, namespace.Name, "rhtas-operator", channel)
		} else {
			extension, catalog, err = olm.OlmInstaller(ctx, cli, baseCatalogImage, namespace.Name, "rhtas-operator", channel, testSupportKubernetes.IsRemoteClusterOpenshift())
		}
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(catalog), catalog.Unwrap())).To(gomega.Succeed())
			return catalog.IsReady(ctx, cli)
		}).Should(gomega.BeTrue())

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(extension), extension.Unwrap())).To(gomega.Succeed())
			return extension.IsReady(ctx, cli)
		}).Should(gomega.BeTrue())

		gomega.Eventually(extension.GetVersion).WithArguments(ctx, cli).Should(gomega.Not(gomega.BeEmpty()))

		baseVersion = extension.GetVersion(ctx, cli)

		gomega.Eventually(func(g gomega.Gomega) []v13.Deployment {
			list := &v13.DeploymentList{}
			g.Expect(cli.List(ctx, list, runtimeCli.InNamespace(namespace.Name), runtimeCli.MatchingLabels{labels.LabelAppPartOf: "rhtas-operator"})).To(gomega.Succeed())
			return list.Items
		}).Should(gomega.And(gomega.HaveLen(1), gomega.WithTransform(func(items []v13.Deployment) int32 {
			return items[0].Status.AvailableReplicas
		}, gomega.BeNumerically(">=", 1))))
	})

	It("Verify CRD REST endpoints", func(ctx SpecContext) {
		gomega.Eventually(func(g gomega.Gomega) {
			tas.VerifyCRDRESTEndpointsForVersion(ctx, cli, v1alpha1.GroupVersion)
		}).Should(gomega.Succeed())
	})

	It("Setup database", func(ctx SpecContext) {
		if fipsEnabled {
			gomega.Expect(postgresql.CreateDB(ctx, cli, namespace.Name, postgresql.DefaultSecretName, "fips-password")).To(gomega.Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespace.Name)
		}
	})

	It("Install securesign", func(ctx SpecContext) {
		metricsAnnotation := "false"
		if testSupportKubernetes.IsRemoteClusterOpenshift() {
			metricsAnnotation = "true"
		}
		securesignDeployment = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: namespace.Name,
				Annotations: map[string]string{
					"rhtas.redhat.com/metrics": metricsAnnotation,
				},
			},
			Spec: v1alpha1.SecuresignSpec{
				Trillian: v1alpha1.TrillianSpec{
					Db: v1alpha1.TrillianDB{
						Create: ptr.To(true),
					},
				},
				Fulcio: v1alpha1.FulcioSpec{
					ExternalAccess: v1alpha1.ExternalAccess{Enabled: true},
					Certificate: v1alpha1.FulcioCert{
						OrganizationName:  "MyOrg",
						OrganizationEmail: "my@email.org",
						CommonName:        "fulcio",
					},
					Config: v1alpha1.FulcioConfig{
						OIDCIssuers: []v1alpha1.OIDCIssuer{
							{
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Issuer:    support.OidcIssuerUrl(),
								Type:      "email",
							},
						},
					},
					Monitoring: v1alpha1.MonitoringConfig{Enabled: true},
				},
				Rekor: v1alpha1.RekorSpec{
					ExternalAccess: v1alpha1.ExternalAccess{Enabled: true},
					RekorSearchUI:  v1alpha1.RekorSearchUI{Enabled: ptr.To(true)},
					Monitoring:     v1alpha1.MonitoringWithTLogConfig{MonitoringConfig: v1alpha1.MonitoringConfig{Enabled: true}},
				},
				Tuf: v1alpha1.TufSpec{
					ExternalAccess: v1alpha1.ExternalAccess{Enabled: true},
				},
				Ctlog: v1alpha1.CTlogSpec{
					Monitoring: v1alpha1.MonitoringWithTLogConfig{MonitoringConfig: v1alpha1.MonitoringConfig{Enabled: true}},
				},
				TimestampAuthority: &v1alpha1.TimestampAuthoritySpec{
					ExternalAccess: v1alpha1.ExternalAccess{Enabled: true},
					Signer: v1alpha1.TimestampAuthoritySigner{
						CertificateChain: v1alpha1.CertificateChain{
							RootCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
							IntermediateCA: []*v1alpha1.TsaCertificateAuthority{
								{
									OrganizationName:  "MyOrg",
									OrganizationEmail: "my@email.org",
									CommonName:        "tsa.hostname",
								},
							},
							LeafCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
						},
					},
					NTPMonitoring: v1alpha1.NTPMonitoring{
						Enabled: true,
						Config: &v1alpha1.NtpMonitoringConfig{
							RequestAttempts: 3,
							RequestTimeout:  5,
							NumServers:      4,
							ServerThreshold: 3,
							MaxTimeDelta:    6,
							Period:          60,
							Servers:         []string{"time.apple.com", "time.google.com", "time-a-b.nist.gov", "time-b-b.nist.gov", "gbg1.ntp.se"},
						},
					},
					Monitoring: v1alpha1.MonitoringConfig{Enabled: true},
				},
			},
		}
		securesignDeployment.Spec.Trillian.Monitoring.Enabled = true

		if fipsEnabled {
			securesignDeployment.Spec.Trillian.Db.Create = ptr.To(false)
			securesignDeployment.Spec.Trillian.Db.Provider = postgresql.Provider
			securesignDeployment.Spec.Trillian.Db.Uri = postgresql.ConnectionURI
			securesignDeployment.Spec.Trillian.Auth = &v1alpha1.Auth{
				Env: postgresql.AuthEnvVars(namespace.Name, postgresql.DefaultSecretName),
			}
		}

		gomega.Expect(cli.Create(ctx, securesignDeployment)).To(gomega.Succeed())

		// Before upgrade, only v1alpha1 CRDs are available — verify using v1alpha1 types
		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(securesignDeployment), securesignDeployment)).To(gomega.Succeed())
			return meta.IsStatusConditionTrue(securesignDeployment.GetConditions(), constants.ReadyCondition)
		}).Should(gomega.BeTrue())

	})

	It("Initialize cosign cli", func(ctx SpecContext) {
		gomega.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(securesignDeployment), securesignDeployment)).To(gomega.Succeed())
		cosign = cosignSupport.NewLocalCosign(securesignDeployment.Status.TufStatus.Url, securesignDeployment.Status.FulcioStatus.Url, securesignDeployment.Status.RekorStatus.Url, securesignDeployment.Status.TSAStatus.Url)
	})

	It("Sign image with cosign cli", func(ctx SpecContext) {
		cosign.VerifyByCosign(ctx, prevImageName)
	})

	It("Upgrade operator", func(ctx SpecContext) {
		gomega.Eventually(func(g gomega.Gomega) error {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(catalog), catalog.Unwrap())).To(gomega.Succeed())
			catalog.UpdateSourceImage(targetedCatalogImage)
			return cli.Update(ctx, catalog.Unwrap())
		}).Should(gomega.Succeed())

		// propagate changes
		time.Sleep(time.Second)

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(catalog), catalog.Unwrap())).To(gomega.Succeed())
			return catalog.IsReady(ctx, cli)
		}).Should(gomega.BeTrue())

		gomega.Eventually(func(g gomega.Gomega) bool {
			g.Expect(cli.Get(ctx, runtimeCli.ObjectKeyFromObject(extension), extension.Unwrap())).To(gomega.Succeed())
			return extension.IsReady(ctx, cli) && extension.GetVersion(ctx, cli) != baseVersion
		}).WithTimeout(5 * time.Minute).Should(gomega.BeTrue())
	})

	It("Verify deployment was upgraded", func(ctx SpecContext) {
		semverBase, err := semver.Make(baseVersion)
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		// Wait for the extension to stabilize after upgrade - OLM may briefly transition CSV states
		gomega.Eventually(func(g gomega.Gomega) {
			version := extension.GetVersion(ctx, cli)
			g.Expect(version).ToNot(gomega.BeEmpty(), "Extension version should not be empty")
			semverNew, err := semver.Make(version)
			g.Expect(err).ToNot(gomega.HaveOccurred())
			g.Expect(semverNew.GT(semverBase)).To(gomega.BeTrue(), "New version should be greater than base version")
		}).Should(gomega.Succeed())

		for k, v := range map[string]string{
			fulcioAction.DeploymentName:            images.Registry.Get(images.FulcioServer),
			ctl.DeploymentName:                     images.Registry.Get(images.CTLog),
			tufAction.DeploymentName:               images.Registry.Get(images.HttpServer),
			rekorAction.ServerDeploymentName:       images.Registry.Get(images.RekorServer),
			rekorAction.SearchUiDeploymentName:     images.Registry.Get(images.RekorSearchUi),
			trillianAction.LogsignerDeploymentName: images.Registry.Get(images.TrillianLogSigner),
			trillianAction.LogserverDeploymentName: images.Registry.Get(images.TrillianServer),
			tsaAction.DeploymentName:               images.Registry.Get(images.TimestampAuthority),
		} {
			gomega.Eventually(func(g gomega.Gomega) string {
				d := &v13.Deployment{}
				g.Expect(cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      k,
				}, d)).To(gomega.Succeed())

				return d.Spec.Template.Spec.Containers[0].Image
			}).Should(gomega.Equal(v), fmt.Sprintf("Expected %s deployment image to be equal to %s", k, v))
		}

		tas.VerifyCRDRESTEndpointsForVersion(ctx, cli, rhtasv1.GroupVersion)

		var v1Securesign *rhtasv1.Securesign
		gomega.Eventually(func() *rhtasv1.Securesign {
			v1Securesign = securesign.Get(ctx, cli, namespace.Name, securesignDeployment.Name)
			return v1Securesign
		}).Should(gomega.Not(gomega.BeNil()))
		tas.VerifyAllComponents(ctx, cli, v1Securesign, !fipsEnabled, true)
	})

	It("Enforce PSA restricted:latest after upgrade", func(ctx SpecContext) {
		support.EnforcePSARestricted(ctx, cli, namespace)
	})

	It("Verify image signature after upgrade", func(ctx SpecContext) {
		gomega.Eventually(func() error {
			return cosign.Verify(ctx, prevImageName)
		}).Should(gomega.Succeed())
	})

	It("Sign and Verify new image after upgrade", func(ctx SpecContext) {
		cosign.VerifyByCosign(ctx, newImageName)
	})

	It("Make sure securesign can be deleted after upgrade", func(ctx SpecContext) {
		gomega.Eventually(func(g gomega.Gomega) {
			s := securesign.Get(ctx, cli, namespace.Name, securesignDeployment.Name)
			gomega.Expect(cli.Delete(ctx, s)).Should(gomega.Succeed())
		}).Should(gomega.Succeed())
	})
})
