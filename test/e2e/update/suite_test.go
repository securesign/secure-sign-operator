//go:build integration

package update

import (
	"context"
	"testing"
	"time"

	"k8s.io/utils/ptr"

	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/test/e2e/support"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
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
				Pvc: rhtasv1alpha1.Pvc{
					Retain: ptr.To(false),
				},
			}},
			TimestampAuthority: rhtasv1alpha1.TimestampAuthoritySpec{
				ExternalAccess: rhtasv1alpha1.ExternalAccess{
					Enabled: true,
				},
				Signer: rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "tsa.hostname",
						},
						IntermediateCA: []rhtasv1alpha1.TsaCertificateAuthority{
							{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
						},
						LeafCA: rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "tsa.hostname",
						},
					},
				},
				NTPMonitoring: rhtasv1alpha1.NTPMonitoring{
					Enabled: true,
					Config: &rhtasv1alpha1.NtpMonitoringConfig{
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
}

func getDeploymentGeneration(ctx context.Context, cli runtimeCli.Client, nn types.NamespacedName) int64 {
	deployment := appsv1.Deployment{}
	if err := cli.Get(ctx, nn, &deployment); err != nil {
		return -1
	}
	return deployment.Status.ObservedGeneration
}
