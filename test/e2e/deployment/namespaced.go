//go:build integration

package deployment

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

var _ = Describe("Install components to separate namespaces", Ordered, func() {
	cli, _ := support.CreateClient()

	var targetImageName string
	namespaces := map[string]*v1.Namespace{
		"rekor":    nil,
		"fulcio":   nil,
		"ctlog":    nil,
		"trillian": nil,
		"tsa":      nil,
		"tuf":      nil,
	}

	var rekorObject *v1alpha1.Rekor
	var fulcioObject *v1alpha1.Fulcio
	var ctlogObject *v1alpha1.CTlog
	var trillianObject *v1alpha1.Trillian
	var tsaObject *v1alpha1.TimestampAuthority
	var tufObject *v1alpha1.Tuf

	BeforeAll(func(ctx SpecContext) {
		DeferCleanup(func(ctx SpecContext) {
			for _, n := range namespaces {
				if n == nil {
					continue
				}
				steps.DumpNamespace(cli, n)(ctx)
				_ = cli.Delete(ctx, n)
			}
		})
		for i := range namespaces {
			namespaces[i] = support.CreateTestNamespace(ctx, cli)
			AddReportEntry(steps.Namespace, namespaces[i].Name)
		}

		trillianObject = &v1alpha1.Trillian{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["trillian"].Name,
				Name:      "test",
			},
			Spec: v1alpha1.TrillianSpec{
				Db: v1alpha1.TrillianDB{Create: ptr.To(true)},
			},
		}

		rekorObject = &v1alpha1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["rekor"].Name,
				Name:      "test",
			},
			Spec: v1alpha1.RekorSpec{
				ExternalAccess: v1alpha1.ExternalAccess{
					Enabled: true,
				},
				Trillian: v1alpha1.TrillianService{
					Address: fmt.Sprintf("trillian-logserver.%s.svc.cluster.local", namespaces["trillian"].Name),
				},
				Signer: v1alpha1.RekorSigner{
					KMS: "secret",
					KeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-rekor-secret",
						},
						Key: "private",
					},
				},
			},
		}

		ctlogObject = &v1alpha1.CTlog{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["ctlog"].Name,
				Name:      "test",
			},
			Spec: v1alpha1.CTlogSpec{
				Trillian: v1alpha1.TrillianService{
					Address: fmt.Sprintf("trillian-logserver.%s.svc.cluster.local", namespaces["trillian"].Name),
				},
				PrivateKeyRef: &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "my-ctlog-secret",
					},
					Key: "private",
				},
				RootCertificates: []v1alpha1.SecretKeySelector{
					{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-fulcio-secret",
						},
						Key: "cert",
					},
				},
			},
		}

		protocol := "http"
		if testSupportKubernetes.IsRemoteClusterOpenshift() {
			// enable TLS
			protocol = "https"
		}

		fulcioObject = &v1alpha1.Fulcio{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["fulcio"].Name,
				Name:      "test",
			},
			Spec: v1alpha1.FulcioSpec{
				Ctlog: v1alpha1.CtlogService{
					Address: fmt.Sprintf("%s://ctlog.%s.svc.cluster.local", protocol, namespaces["ctlog"].Name),
				},
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
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-fulcio-secret",
						},
						Key: "private",
					},
					PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-fulcio-secret",
						},
						Key: "password",
					},
					CARef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1alpha1.LocalObjectReference{
							Name: "my-fulcio-secret",
						},
						Key: "cert",
					},
				},
			},
		}

		tsaObject = &v1alpha1.TimestampAuthority{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["tsa"].Name,
				Name:      "test",
			},
			Spec: v1alpha1.TimestampAuthoritySpec{
				ExternalAccess: v1alpha1.ExternalAccess{
					Enabled: true,
				},
				Signer: v1alpha1.TimestampAuthoritySigner{
					CertificateChain: v1alpha1.CertificateChain{
						CertificateChainRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-tsa-secret",
							},
							Key: "certificateChain",
						},
					},
					File: &v1alpha1.File{
						PrivateKeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-tsa-secret",
							},
							Key: "leafPrivateKey",
						},
						PasswordRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-tsa-secret",
							},
							Key: "leafPrivateKeyPassword",
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
		}

		tufObject = &v1alpha1.Tuf{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["tuf"].Name,
				Name:      "test",
			},
			Spec: v1alpha1.TufSpec{
				ExternalAccess: v1alpha1.ExternalAccess{
					Enabled: true,
				},
				Keys: []v1alpha1.TufKey{
					{
						Name: "fulcio_v1.crt.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					},
					{
						Name: "rekor.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-rekor-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "ctfe.pub",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "my-ctlog-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "tsa.certchain.pem",
						SecretRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1alpha1.LocalObjectReference{
								Name: "test-tsa-secret",
							},
							Key: "certificateChain",
						},
					},
				},
			},
		}
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with autogenerated certificates", func() {
		BeforeAll(func(ctx SpecContext) {
			By("stores secrets into namespaces")
			// Rekor
			rekorSecret := rekor.CreateSecret(namespaces["rekor"].Name, "my-rekor-secret")

			tufRekorSecret := rekorSecret.DeepCopy()
			tufRekorSecret.Namespace = namespaces["tuf"].Name

			Expect(cli.Create(ctx, rekorSecret)).To(Succeed())
			Expect(cli.Create(ctx, tufRekorSecret)).To(Succeed())

			// Fulcio
			fulcioSecret := fulcio.CreateSecret(namespaces["fulcio"].Name, "my-fulcio-secret")

			tufFulcioSecret := fulcioSecret.DeepCopy()
			tufFulcioSecret.Namespace = namespaces["tuf"].Name

			ctlogRootCASecret := fulcioSecret.DeepCopy()
			ctlogRootCASecret.Namespace = namespaces["ctlog"].Name

			Expect(cli.Create(ctx, fulcioSecret)).To(Succeed())
			Expect(cli.Create(ctx, tufFulcioSecret)).To(Succeed())
			Expect(cli.Create(ctx, ctlogRootCASecret)).To(Succeed())

			// Ctlog
			ctlogSecret := ctlog.CreateSecret(namespaces["ctlog"].Name, "my-ctlog-secret")

			tufCtlogSecret := ctlogSecret.DeepCopy()
			tufCtlogSecret.Namespace = namespaces["tuf"].Name

			Expect(cli.Create(ctx, ctlogSecret)).To(Succeed())
			Expect(cli.Create(ctx, tufCtlogSecret)).To(Succeed())

			// TSA
			tsaSecret := tsa.CreateSecrets(namespaces["tsa"].Name, "test-tsa-secret")

			tufTSASecret := tsaSecret.DeepCopy()
			tufTSASecret.Namespace = namespaces["tuf"].Name

			Expect(cli.Create(ctx, tsaSecret)).To(Succeed())
			Expect(cli.Create(ctx, tufTSASecret)).To(Succeed())

			By("create components")
			Expect(cli.Create(ctx, trillianObject)).To(Succeed())
			Expect(cli.Create(ctx, rekorObject)).To(Succeed())
			Expect(cli.Create(ctx, fulcioObject)).To(Succeed())
			Expect(cli.Create(ctx, ctlogObject)).To(Succeed())
			Expect(cli.Create(ctx, tsaObject)).To(Succeed())
			Expect(cli.Create(ctx, tufObject)).To(Succeed())
		})

		It("All other components are running", func(ctx SpecContext) {
			trillian.Verify(ctx, cli, namespaces["trillian"].Name, trillianObject.Name, true)
			rekor.Verify(ctx, cli, namespaces["rekor"].Name, rekorObject.Name, true)
			fulcio.Verify(ctx, cli, namespaces["fulcio"].Name, fulcioObject.Name)
			ctlog.Verify(ctx, cli, namespaces["ctlog"].Name, ctlogObject.Name)
			tsa.Verify(ctx, cli, namespaces["tsa"].Name, tsaObject.Name)
			tuf.Verify(ctx, cli, namespaces["tuf"].Name, tufObject.Name)
		})

		It("Use cosign cli", func(ctx SpecContext) {
			f := fulcio.Get(ctx, cli, namespaces["fulcio"].Name, fulcioObject.Name)
			Expect(f).ToNot(BeNil())

			r := rekor.Get(ctx, cli, namespaces["rekor"].Name, rekorObject.Name)
			Expect(r).ToNot(BeNil())

			t := tuf.Get(ctx, cli, namespaces["tuf"].Name, tufObject.Name)
			Expect(t).ToNot(BeNil())

			ts := tsa.Get(ctx, cli, namespaces["tsa"].Name, tsaObject.Name)
			Expect(ts).ToNot(BeNil())

			tas.VerifyByCosignCustom(ctx, cli, f, r, t, ts, targetImageName)
		})
	})
})
