//go:build integration

package e2e

import (
	"context"
	"time"

	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"

	"github.com/securesign/operator/internal/controller/common/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Securesign install with provided certs", Ordered, func() {
	cli, _ := CreateClient()
	ctx := context.TODO()

	var targetImageName string
	var namespace *v1.Namespace
	var s *v1alpha1.Securesign

	AfterEach(func() {
		if CurrentSpecReport().Failed() && support.IsCIEnvironment() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			_ = cli.Delete(ctx, namespace)
		})

		s = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
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
				Ctlog: v1alpha1.CTlogSpec{
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
				Tuf: v1alpha1.TufSpec{
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
					},
				},
				Trillian: v1alpha1.TrillianSpec{Db: v1alpha1.TrillianDB{
					Create: utils.Pointer(true),
				}},
			},
		}
	})

	BeforeAll(func() {
		targetImageName = support.PrepareImage(ctx)
	})

	Describe("Install with provided certificates", func() {
		BeforeAll(func() {
			Expect(cli.Create(ctx, ctlog.CreateSecret(namespace.Name, "my-ctlog-secret"))).To(Succeed())
			Expect(cli.Create(ctx, fulcio.CreateSecret(namespace.Name, "my-fulcio-secret"))).To(Succeed())
			Expect(cli.Create(ctx, rekor.CreateSecret(namespace.Name, "my-rekor-secret"))).To(Succeed())
			Expect(cli.Create(ctx, s)).To(Succeed())
		})

		It("fulcio is running with mounted certs", func() {
			fulcio.Verify(ctx, cli, namespace.Name, s.Name)
			server := fulcio.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())

			sp := []v1.SecretProjection{}
			for _, volume := range server.Spec.Volumes {
				if volume.Name == "fulcio-cert" {
					for _, source := range volume.VolumeSource.Projected.Sources {
						sp = append(sp, *source.Secret)
					}
				}
			}

			Expect(sp).To(
				ContainElement(
					WithTransform(func(sp v1.SecretProjection) string {
						return sp.Name
					}, Equal("my-fulcio-secret")),
				))

		})

		It("rekor is running with mounted certs", func() {
			rekor.Verify(ctx, cli, namespace.Name, s.Name)
			server := rekor.GetServerPod(ctx, cli, namespace.Name)()
			Expect(server).NotTo(BeNil())
			Expect(server.Spec.Volumes).To(
				ContainElement(
					WithTransform(func(volume v1.Volume) string {
						if volume.VolumeSource.Secret != nil {
							return volume.VolumeSource.Secret.SecretName
						}
						return ""
					}, Equal("my-rekor-secret")),
				))

		})

		It("All other components are running", func() {
			securesign.Verify(ctx, cli, namespace.Name, s.Name)
			ctlog.Verify(ctx, cli, namespace.Name, s.Name)
			trillian.Verify(ctx, cli, namespace.Name, s.Name, true)
			tuf.Verify(ctx, cli, namespace.Name, s.Name)
		})

		It("Use cosign cli", func() {
			fulcio := fulcio.Get(ctx, cli, namespace.Name, s.Name)()
			Expect(fulcio).ToNot(BeNil())

			rekor := rekor.Get(ctx, cli, namespace.Name, s.Name)()
			Expect(rekor).ToNot(BeNil())

			tuf := tuf.Get(ctx, cli, namespace.Name, s.Name)()
			Expect(tuf).ToNot(BeNil())

			oidcToken, err := support.OidcToken(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(oidcToken).ToNot(BeEmpty())

			// sleep for a while to be sure everything has settled down
			time.Sleep(time.Duration(10) * time.Second)

			Expect(clients.Execute("cosign", "initialize", "--mirror="+tuf.Status.Url, "--root="+tuf.Status.Url+"/root.json")).To(Succeed())

			Expect(clients.Execute(
				"cosign", "sign", "-y",
				"--fulcio-url="+fulcio.Status.Url,
				"--rekor-url="+rekor.Status.Url,
				"--oidc-issuer="+support.OidcIssuerUrl(),
				"--oidc-client-id="+support.OidcClientID(),
				"--identity-token="+oidcToken,
				targetImageName,
			)).To(Succeed())

			Expect(clients.Execute(
				"cosign", "verify",
				"--rekor-url="+rekor.Status.Url,
				"--certificate-identity-regexp", ".*@redhat",
				"--certificate-oidc-issuer-regexp", ".*keycloak.*",
				targetImageName,
			)).To(Succeed())
		})
	})
})
