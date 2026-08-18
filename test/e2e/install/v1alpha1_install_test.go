//go:build integration

package install

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("Securesign install with v1alpha1 API", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign
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
		s = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: namespace.Name,
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
				},
				Rekor: v1alpha1.RekorSpec{
					ExternalAccess: v1alpha1.ExternalAccess{Enabled: true},
				},
				Tuf: v1alpha1.TufSpec{
					ExternalAccess: v1alpha1.ExternalAccess{Enabled: true},
				},
				Ctlog: v1alpha1.CTlogSpec{},
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
				},
			},
		}

		if fipsEnabled {
			s.Spec.Trillian.Db.Create = ptr.To(false)
			s.Spec.Trillian.Db.Provider = postgresql.Provider
			s.Spec.Trillian.Db.Uri = postgresql.ConnectionURI
			s.Spec.Trillian.Auth = &v1alpha1.Auth{
				Env: postgresql.AuthEnvVars(namespace.Name, postgresql.DefaultSecretName),
			}
		}
	})

	BeforeAll(func(ctx SpecContext) {
		reg := support.DeployTestRegistry(ctx, cli, namespace.Name)
		DeferCleanup(reg.Close)
		targetImageName = reg.PrepareImage(ctx)
	})

	Describe("Install with v1alpha1 CR", func() {
		BeforeAll(func(ctx SpecContext) {
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("All components are running", func(ctx SpecContext) {
			v1Instance := securesign.Get(ctx, cli, namespace.Name, s.Name)
			Expect(v1Instance).ToNot(BeNil())
			tas.VerifyAllComponents(ctx, cli, v1Instance, !fipsEnabled, true)
		})

		It("Use cosign cli", func(ctx SpecContext) {
			v1Instance := securesign.Get(ctx, cli, namespace.Name, s.Name)
			Expect(v1Instance).ToNot(BeNil())
			tas.VerifyByCosign(ctx, targetImageName, v1Instance.Status.TufStatus.Url, v1Instance.Status.FulcioStatus.Url, v1Instance.Status.RekorStatus.Url, v1Instance.Status.TSAStatus.Url)
		})

		It("v1alpha1 status is populated", func(ctx SpecContext) {
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), s)).To(Succeed())
			Expect(s.Status.RekorStatus.Url).ToNot(BeEmpty())
			Expect(s.Status.FulcioStatus.Url).ToNot(BeEmpty())
			Expect(s.Status.TufStatus.Url).ToNot(BeEmpty())
			Expect(s.Status.TSAStatus.Url).ToNot(BeEmpty())
		})

		It("v1 view is consistent", func(ctx SpecContext) {
			v1Instance := &rhtasv1.Securesign{}
			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), v1Instance)).To(Succeed())

			Expect(cli.Get(ctx, nsName(namespace.Name, s.Name), s)).To(Succeed())
			Expect(v1Instance.Status.RekorStatus.Url).To(Equal(s.Status.RekorStatus.Url))
			Expect(v1Instance.Status.FulcioStatus.Url).To(Equal(s.Status.FulcioStatus.Url))
			Expect(v1Instance.Status.TufStatus.Url).To(Equal(s.Status.TufStatus.Url))
			Expect(v1Instance.Status.TSAStatus.Url).To(Equal(s.Status.TSAStatus.Url + rhtasv1.TimestampPath))
		})
	})
})
