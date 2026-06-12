//go:build integration

package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const NamespaceMask = "benchmark-install-%d-"

func BenchmarkInstall(b *testing.B) {
	gomega.RegisterTestingT(b)
	gomega.SetDefaultEventuallyTimeout(3 * time.Minute)
	log.SetLogger(ginkgo.GinkgoLogr)

	cli, err := support.CreateClient()
	if err != nil {
		b.Fatalf("could not create client: %v", err)
	}

	loop := func(iteration int) {
		var (
			namespaceName   string
			ctx             = context.Background()
			err             error
			targetImageName string
		)

		namespaceName, err = createNamespace(ctx, cli, iteration)
		if err != nil {
			b.Fatalf("could not create namespace: %v", err)
		}
		defer deleteNamespace(ctx, cli, namespaceName)
		defer dumpNamespace(ctx, cli, b, namespaceName)

		targetImageName = support.PrepareImage(context.Background())

		b.StartTimer()
		err = installTAS(ctx, cli, namespaceName)
		b.StopTimer()

		if err != nil {
			b.Fatalf("could not install: %v", err)
		}
		s := securesign.Get(ctx, cli, namespaceName, "test")
		tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop(i)
	}
}

func createNamespace(ctx context.Context, cli client.Client, iteration int) (string, error) {
	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(NamespaceMask, iteration),
		},
	}
	err := cli.Create(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("creating namespace: %w", err)
	}
	return namespace.Name, nil
}

func installTAS(ctx context.Context, cli client.Client, namespace string) error {
	instance := &rhtasv1.Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test",
		},
		Spec: rhtasv1.SecuresignSpec{
			Rekor: rhtasv1.RekorSpec{
				ExternalAccess: rhtasv1.ExternalAccess{
					Enabled: true,
				},
				RekorSearchUI: rhtasv1.RekorSearchUI{
					Enabled: utils.Pointer(true),
				},
			},
			Fulcio: rhtasv1.FulcioSpec{
				ExternalAccess: rhtasv1.ExternalAccess{
					Enabled: true,
				},
				Config: rhtasv1.FulcioConfig{
					OIDCIssuers: []rhtasv1.OIDCIssuer{
						{
							ClientID:  support.OidcClientID(),
							IssuerURL: support.OidcIssuerUrl(),
							Issuer:    support.OidcIssuerUrl(),
							Type:      "email",
						},
					}},
				Certificate: rhtasv1.FulcioCert{
					OrganizationName:  "MyOrg",
					OrganizationEmail: "my@email.org",
					CommonName:        "fulcio",
				},
			},
			Ctlog: rhtasv1.CTlogSpec{},
			Tuf: rhtasv1.TufSpec{
				ExternalAccess: rhtasv1.ExternalAccess{
					Enabled: true,
				},
			},
			Trillian: rhtasv1.TrillianSpec{Db: rhtasv1.TrillianDB{
				Create: ptr.To(true),
			}},
			TimestampAuthority: &rhtasv1.TimestampAuthoritySpec{
				ExternalAccess: rhtasv1.ExternalAccess{
					Enabled: true,
				},
				Signer: rhtasv1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1.CertificateChain{
						RootCA: &rhtasv1.TsaCertificateAuthority{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "tsa.hostname",
						},
						IntermediateCA: []*rhtasv1.TsaCertificateAuthority{
							{
								OrganizationName:  "MyOrg",
								OrganizationEmail: "my@email.org",
								CommonName:        "tsa.hostname",
							},
						},
						LeafCA: &rhtasv1.TsaCertificateAuthority{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.org",
							CommonName:        "tsa.hostname",
						},
					},
				},
			},
		},
	}

	if err := cli.Create(ctx, instance); err != nil {
		return fmt.Errorf("creating instance: %w", err)
	}

	tas.VerifyAllComponents(ctx, cli, instance, true)

	return nil
}

func deleteNamespace(ctx context.Context, cli client.Client, namespace string) {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	_ = cli.Delete(ctx, ns)
}

func dumpNamespace(ctx context.Context, cli client.Client, b *testing.B, namespace string) {
	if b.Failed() && support.IsCIEnvironment() {
		support.DumpNamespace(ctx, cli, namespace)
	}
}
