//go:build integration

package install

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/test/e2e/support"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/postgresql"
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
	var fipsEnabled bool
	namespaces := map[string]*v1.Namespace{
		"rekor":    nil,
		"fulcio":   nil,
		"ctlog":    nil,
		"trillian": nil,
		"tsa":      nil,
		"tuf":      nil,
	}

	var rekorObject *rhtasv1.Rekor
	var fulcioObject *rhtasv1.Fulcio
	var ctlogObject *rhtasv1.CTlog
	var trillianObject *rhtasv1.Trillian
	var tsaObject *rhtasv1.TimestampAuthority
	var tufObject *rhtasv1.Tuf

	BeforeAll(steps.DetectAndConfigureFIPS(cli, func(enabled bool) {
		fipsEnabled = enabled
	}))

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

		if fipsEnabled {
			Expect(postgresql.CreateDB(ctx, cli, namespaces["trillian"].Name, postgresql.DefaultSecretName, "fips-password")).To(Succeed())
			postgresql.WaitAndLoadSchema(ctx, cli, namespaces["trillian"].Name)
			trillianObject = &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespaces["trillian"].Name,
					Name:      "test",
				},
				Spec: rhtasv1.TrillianSpec{
					Auth: &rhtasv1.Auth{
						Env: postgresql.AuthEnvVars(namespaces["trillian"].Name, postgresql.DefaultSecretName),
					},
					Db: rhtasv1.TrillianDB{
						Create:   ptr.To(false),
						Provider: postgresql.Provider,
						Uri:      postgresql.ConnectionURI,
					},
				},
			}
		} else {
			trillianObject = &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespaces["trillian"].Name,
					Name:      "test",
				},
				Spec: rhtasv1.TrillianSpec{
					Db: rhtasv1.TrillianDB{Create: ptr.To(true)},
				},
			}
		}

		rekorObject = &rhtasv1.Rekor{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["rekor"].Name,
				Name:      "test",
			},
			Spec: rhtasv1.RekorSpec{
				Ingress: rhtasv1.Ingress{
					Enabled: ptr.To(true),
				},
				Trillian: rhtasv1.ServiceReference{
					URL: fmt.Sprintf("trillian-logserver.%s.svc.cluster.local:8091", namespaces["trillian"].Name),
				},
				Signer: rhtasv1.RekorSigner{
					KMS: "secret",
					KeyRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{
							Name: "my-rekor-secret",
						},
						Key: "private",
					},
				},
			},
		}

		ctlogObject = &rhtasv1.CTlog{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["ctlog"].Name,
				Name:      "test",
			},
			Spec: rhtasv1.CTlogSpec{
				Trillian: rhtasv1.ServiceReference{
					Ref: &rhtasv1.ServiceReferenceRef{
						Name:      "test",
						Namespace: namespaces["trillian"].Name,
					},
				},
				PrivateKeyRef: &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "my-ctlog-secret",
					},
					Key: "private",
				},
				RootCertificates: []rhtasv1.SecretKeySelector{
					{
						LocalObjectReference: rhtasv1.LocalObjectReference{
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

		fulcioObject = &rhtasv1.Fulcio{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["fulcio"].Name,
				Name:      "test",
			},
			Spec: rhtasv1.FulcioSpec{
				Ctlog: rhtasv1.CtlogService{
					Address: fmt.Sprintf("%s://ctlog.%s.svc.cluster.local", protocol, namespaces["ctlog"].Name),
				},
				Ingress: rhtasv1.Ingress{
					Enabled: ptr.To(true),
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
				Certificate: func() rhtasv1.FulcioCert {
					cert := rhtasv1.FulcioCert{
						PrivateKeyRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "private",
						},
						CARef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					}
					if !fipsEnabled {
						cert.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{ //nolint:staticcheck
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "password",
						}
					}
					return cert
				}(),
			},
		}

		tsaObject = &rhtasv1.TimestampAuthority{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["tsa"].Name,
				Name:      "test",
			},
			Spec: rhtasv1.TimestampAuthoritySpec{
				Ingress: rhtasv1.Ingress{
					Enabled: ptr.To(true),
				},
				Signer: func() rhtasv1.TimestampAuthoritySigner {
					signer := rhtasv1.TimestampAuthoritySigner{
						CertificateChain: rhtasv1.CertificateChain{
							CertificateChainRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{
									Name: "test-tsa-secret",
								},
								Key: "certificateChain",
							},
						},
						File: &rhtasv1.File{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{
									Name: "test-tsa-secret",
								},
								Key: "leafPrivateKey",
							},
						},
					}
					if !fipsEnabled {
						signer.File.PasswordRef = &rhtasv1.SecretKeySelector{ //nolint:staticcheck
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "test-tsa-secret",
							},
							Key: "leafPrivateKeyPassword",
						}
					}
					return signer
				}(),
				NTPMonitoring: rhtasv1.NTPMonitoring{
					Enabled: ptr.To(true),
					Config: &rhtasv1.NtpMonitoringConfig{
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

		tufObject = &rhtasv1.Tuf{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespaces["tuf"].Name,
				Name:      "test",
			},
			Spec: rhtasv1.TufSpec{
				Ingress: rhtasv1.Ingress{
					Enabled: ptr.To(true),
				},
				Keys: []rhtasv1.TufKey{
					{
						Name: "fulcio_v1.crt.pem",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					},
					{
						Name: "rekor.pub",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-rekor-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "ctfe.pub",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
								Name: "my-ctlog-secret",
							},
							Key: "public",
						},
					},
					{
						Name: "tsa.certchain.pem",
						SecretRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{
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
			rekorSecret := rekor.CreateSecret(namespaces["rekor"].Name, "my-rekor-secret", false)

			tufRekorSecret := rekorSecret.DeepCopy()
			tufRekorSecret.Namespace = namespaces["tuf"].Name

			Expect(cli.Create(ctx, rekorSecret)).To(Succeed())
			Expect(cli.Create(ctx, tufRekorSecret)).To(Succeed())

			// Fulcio
			fulcioSecret := fulcio.CreateSecret(namespaces["fulcio"].Name, "my-fulcio-secret", !fipsEnabled)

			tufFulcioSecret := fulcioSecret.DeepCopy()
			tufFulcioSecret.Namespace = namespaces["tuf"].Name

			ctlogRootCASecret := fulcioSecret.DeepCopy()
			ctlogRootCASecret.Namespace = namespaces["ctlog"].Name

			Expect(cli.Create(ctx, fulcioSecret)).To(Succeed())
			Expect(cli.Create(ctx, tufFulcioSecret)).To(Succeed())
			Expect(cli.Create(ctx, ctlogRootCASecret)).To(Succeed())

			// Ctlog
			ctlogSecret := ctlog.CreateSecret(namespaces["ctlog"].Name, "my-ctlog-secret", false)

			tufCtlogSecret := ctlogSecret.DeepCopy()
			tufCtlogSecret.Namespace = namespaces["tuf"].Name

			Expect(cli.Create(ctx, ctlogSecret)).To(Succeed())
			Expect(cli.Create(ctx, tufCtlogSecret)).To(Succeed())

			// TSA
			tsaSecret := tsa.CreateSecrets(namespaces["tsa"].Name, "test-tsa-secret", !fipsEnabled)

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
		})

		It("All other components are running", func(ctx SpecContext) {
			trillian.Verify(ctx, cli, namespaces["trillian"].Name, trillianObject.Name, !fipsEnabled)
			rekor.Verify(ctx, cli, namespaces["rekor"].Name, rekorObject.Name, true)
			fulcio.Verify(ctx, cli, namespaces["fulcio"].Name, fulcioObject.Name)
			ctlog.Verify(ctx, cli, namespaces["ctlog"].Name, ctlogObject.Name)
			tsa.Verify(ctx, cli, namespaces["tsa"].Name, tsaObject.Name)

		})

		It("Create TUF instance", func(ctx SpecContext) {
			tufObject.Spec.Fulcio.URL = fulcio.Get(ctx, cli, namespaces["fulcio"].Name, fulcioObject.Name).Status.Url
			tufObject.Spec.Rekor.URL = rekor.Get(ctx, cli, namespaces["rekor"].Name, rekorObject.Name).Status.Url
			tufObject.Spec.Tsa.URL = tsa.Get(ctx, cli, namespaces["tsa"].Name, tsaObject.Name).Status.Url
			tufObject.Spec.Ctlog.URL = ctlog.Get(ctx, cli, namespaces["ctlog"].Name, ctlogObject.Name).Status.Url
			Expect(cli.Create(ctx, tufObject)).To(Succeed())
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

			tas.VerifyByCosign(ctx, targetImageName, t.Status.Url, f.Status.Url, r.Status.Url, ts.Status.Url)
		})
	})
})
