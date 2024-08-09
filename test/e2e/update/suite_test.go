//go:build integration

package update

import (
	"context"
	"testing"
	"time"

	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/test/e2e/support"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	routev1 "github.com/openshift/api/route/v1"
	olm "github.com/operator-framework/api/pkg/operators/v1"
	olmAlpha "github.com/operator-framework/api/pkg/operators/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func TestUpdateComponents(t *testing.T) {
	RegisterFailHandler(Fail)
	log.SetLogger(GinkgoLogr)
	SetDefaultEventuallyTimeout(time.Duration(1) * time.Minute)
	RunSpecs(t, "Update components E2E Suite")

	// print whole stack in case of failure
	format.MaxLength = 0
}

func CreateClient() (runtimeCli.Client, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(rhtasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(olmAlpha.AddToScheme(scheme))
	utilruntime.Must(olm.AddToScheme(scheme))

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	return runtimeCli.New(cfg, runtimeCli.Options{Scheme: scheme})
}

func securesignResource(namespace *v1.Namespace) *rhtasv1alpha1.Securesign {
	return &rhtasv1alpha1.Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace.Name,
			Name:      "test",
			Annotations: map[string]string{
				"rhtas.redhat.com/metrics": "false",
			},
		},
		Spec: rhtasv1alpha1.SecuresignSpec{
			Rekor: rhtasv1alpha1.RekorSpec{
				ExternalAccess: rhtasv1alpha1.ExternalAccess{
					Enabled: true,
				},
				RekorSearchUI: rhtasv1alpha1.RekorSearchUI{
					Enabled: utils.Pointer(false),
				},
			},
			Fulcio: rhtasv1alpha1.FulcioSpec{
				ExternalAccess: rhtasv1alpha1.ExternalAccess{
					Enabled: true,
				},
				Config: rhtasv1alpha1.FulcioConfig{
					OIDCIssuers: []rhtasv1alpha1.OIDCIssuer{
						{
							ClientID:  support.OidcClientID(),
							IssuerURL: support.OidcIssuerUrl(),
							Issuer:    support.OidcIssuerUrl(),
							Type:      "email",
						},
					}},
				Certificate: rhtasv1alpha1.FulcioCert{
					OrganizationName:  "MyOrg",
					OrganizationEmail: "my@email.org",
					CommonName:        "fulcio",
				},
			},
			Ctlog: rhtasv1alpha1.CTlogSpec{},
			Tuf: rhtasv1alpha1.TufSpec{
				ExternalAccess: rhtasv1alpha1.ExternalAccess{
					Enabled: true,
				},
			},
			Trillian: rhtasv1alpha1.TrillianSpec{Db: rhtasv1alpha1.TrillianDB{
				Create: utils.Pointer(true),
			}},
		},
	}
}

func getDeploymentGeneration(ctx context.Context, cli runtimeCli.Client, nn types.NamespacedName) int64 {
	deployment := appsv1.Deployment{}
	if err := cli.Get(ctx, nn, &deployment); err != nil {
		return -1
	}
	return deployment.Status.ObservedGeneration
}
func verifyAllComponents(ctx context.Context, cli runtimeCli.Client, s *rhtasv1alpha1.Securesign) {
	securesign.Verify(ctx, cli, s.Namespace, s.Name)
	trillian.Verify(ctx, cli, s.Namespace, s.Name, true)
	ctlog.Verify(ctx, cli, s.Namespace, s.Name)
	tuf.Verify(ctx, cli, s.Namespace, s.Name)
	rekor.Verify(ctx, cli, s.Namespace, s.Name)
	fulcio.Verify(ctx, cli, s.Namespace, s.Name)
}

func verifyByCosign(ctx context.Context, cli runtimeCli.Client, s *rhtasv1alpha1.Securesign, targetImageName string) {
	f := fulcio.Get(ctx, cli, s.Namespace, s.Name)()
	Expect(f).ToNot(BeNil())

	r := rekor.Get(ctx, cli, s.Namespace, s.Name)()
	Expect(r).ToNot(BeNil())

	t := tuf.Get(ctx, cli, s.Namespace, s.Name)()
	Expect(t).ToNot(BeNil())

	oidcToken, err := support.OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

	// sleep for a while to be sure everything has settled down
	time.Sleep(time.Duration(10) * time.Second)

	Expect(clients.Execute("cosign", "initialize", "--mirror="+t.Status.Url, "--root="+t.Status.Url+"/root.json")).To(Succeed())

	Expect(clients.Execute(
		"cosign", "sign", "-y",
		"--fulcio-url="+f.Status.Url,
		"--rekor-url="+r.Status.Url,
		"--oidc-issuer="+support.OidcIssuerUrl(),
		"--oidc-client-id="+support.OidcClientID(),
		"--identity-token="+oidcToken,
		targetImageName,
	)).To(Succeed())

	Expect(clients.Execute(
		"cosign", "verify",
		"--rekor-url="+r.Status.Url,
		"--certificate-identity-regexp", ".*@redhat",
		"--certificate-oidc-issuer-regexp", ".*keycloak.*",
		targetImageName,
	)).To(Succeed())
}
