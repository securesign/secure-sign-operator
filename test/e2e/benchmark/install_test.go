//go:build integration

package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/tas"
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
		tas.VerifyByCosign(ctx, cli, &v1alpha1.Securesign{ObjectMeta: metav1.ObjectMeta{Namespace: namespaceName, Name: "test"}}, targetImageName)
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
	instance := &v1alpha1.Securesign{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "test",
			Annotations: map[string]string{
				"rhtas.redhat.com/metrics": "false",
			},
		},
		Spec: v1alpha1.SecuresignSpec{
			Rekor: v1alpha1.RekorSpec{
				ExternalAccess: v1alpha1.ExternalAccess{
					Enabled: true,
				},
				RekorSearchUI: v1alpha1.RekorSearchUI{
					Enabled: utils.Pointer(true),
				},
			},
			Fulcio: v1alpha1.FulcioSpec{
				ExternalAccess: v1alpha1.ExternalAccess{
					Enabled: true,
				},
				Config: v1alpha1.FulcioConfig{
					OIDCIssuers: []v1alpha1.OIDCIssuer{
						{
							ClientID:  support.OidcClientID(),
							IssuerURL: support.OidcIssuerUrl(),
							Issuer:    support.OidcIssuerUrl(),
							Type:      "email",
						},
					}},
				Certificate: v1alpha1.FulcioCert{
					OrganizationName:  "MyOrg",
					OrganizationEmail: "my@email.org",
					CommonName:        "fulcio",
				},
			},
			Ctlog: v1alpha1.CTlogSpec{},
			Tuf: v1alpha1.TufSpec{
				ExternalAccess: v1alpha1.ExternalAccess{
					Enabled: true,
				},
			},
			Trillian: v1alpha1.TrillianSpec{Db: v1alpha1.TrillianDB{
				Create: ptr.To(true),
			}},
			TimestampAuthority: &v1alpha1.TimestampAuthoritySpec{
				ExternalAccess: v1alpha1.ExternalAccess{
					Enabled: true,
				},
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
