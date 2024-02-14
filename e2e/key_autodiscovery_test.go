//go:build integration

package e2e_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/e2e/support"
	"github.com/securesign/operator/e2e/support/tas"
	clients "github.com/securesign/operator/e2e/support/tas/cli"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Securesign key autodiscovery test", Ordered, func() {
	cli, _ := CreateClient()
	ctx := context.TODO()

	targetImageName := "ttl.sh/" + uuid.New().String() + ":5m"
	var namespace *v1.Namespace
	var securesign *v1alpha1.Securesign

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			support.DumpNamespace(ctx, cli, namespace.Name)
		}
	})

	BeforeAll(func() {
		namespace = support.CreateTestNamespace(ctx, cli)
		DeferCleanup(func() {
			cli.Delete(ctx, namespace)
		})

		securesign = &v1alpha1.Securesign{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "test",
			},
			Spec: v1alpha1.SecuresignSpec{
				Rekor: v1alpha1.RekorSpec{
					ExternalAccess: v1alpha1.ExternalAccess{
						Enabled: true,
					},
					Signer: v1alpha1.RekorSigner{
						KMS: "secret",
						KeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
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
						OIDCIssuers: map[string]v1alpha1.OIDCIssuer{
							support.OidcIssuerUrl(): {
								ClientID:  support.OidcClientID(),
								IssuerURL: support.OidcIssuerUrl(),
								Type:      "email",
							},
						}},
					Certificate: v1alpha1.FulcioCert{
						PrivateKeyRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "private",
						},
						PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "password",
						},
						CARef: &v1alpha1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: "my-fulcio-secret",
							},
							Key: "cert",
						},
					},
				},
				Ctlog: v1alpha1.CTlogSpec{
					PrivateKeyRef: &v1alpha1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: "my-ctlog-secret",
						},
						Key: "private",
					},
					RootCertificates: []v1alpha1.SecretKeySelector{
						{
							LocalObjectReference: v1.LocalObjectReference{
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
				},
				Trillian: v1alpha1.TrillianSpec{Db: v1alpha1.TrillianDB{
					Create: true,
				}},
			},
		}
	})

	BeforeAll(func() {
		support.PrepareImage(ctx, targetImageName)
	})

	Describe("Install with provided certificates", func() {
		BeforeAll(func() {
			Expect(cli.Create(ctx, initCTSecret(namespace.Name, "my-ctlog-secret")))
			Expect(cli.Create(ctx, initFulcioSecret(namespace.Name, "my-fulcio-secret")))
			Expect(cli.Create(ctx, initRekorSecret(namespace.Name, "my-rekor-secret")))
			Expect(cli.Create(ctx, securesign)).To(Succeed())
		})

		It("All components are running", func() {
			tas.VerifyRekor(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyFulcio(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyCTLog(ctx, cli, namespace.Name, securesign.Name)
			tas.VerifyTrillian(ctx, cli, namespace.Name, securesign.Name, true)
			tas.VerifyTuf(ctx, cli, namespace.Name, securesign.Name)
		})

		It("Verify TUF keys", func() {
			tuf := tas.GetTuf(ctx, cli, namespace.Name, securesign.Name)()
			Expect(tuf.Spec.Keys).To(HaveEach(WithTransform(func(k v1alpha1.TufKey) string { return k.SecretRef.Name }, Not(BeEmpty()))))
			var (
				expected, actual []byte
				err              error
			)
			for _, k := range tuf.Spec.Keys {
				actual, err = kubernetes.GetSecretData(cli, namespace.Name, k.SecretRef)
				Expect(err).To(Not(HaveOccurred()))

				switch k.Name {
				case "fulcio_v1.crt.pem":
					expected, err = kubernetes.GetSecretData(cli, namespace.Name, securesign.Spec.Fulcio.Certificate.CARef)
					Expect(err).To(Not(HaveOccurred()))
					break
				case "rekor.pub":
					expectedKeyRef := securesign.Spec.Rekor.Signer.KeyRef.DeepCopy()
					expectedKeyRef.Key = "public"
					expected, err = kubernetes.GetSecretData(cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
					break
				case "ctfe.pub":
					expectedKeyRef := securesign.Spec.Ctlog.PrivateKeyRef.DeepCopy()
					expectedKeyRef.Key = "public"
					expected, err = kubernetes.GetSecretData(cli, namespace.Name, expectedKeyRef)
					Expect(err).To(Not(HaveOccurred()))
					break
				}
				Expect(expected).To(Equal(actual))
			}
		})

		It("Use cosign cli", func() {
			fulcio := tas.GetFulcio(ctx, cli, namespace.Name, securesign.Name)()
			Expect(fulcio).ToNot(BeNil())

			rekor := tas.GetRekor(ctx, cli, namespace.Name, securesign.Name)()
			Expect(rekor).ToNot(BeNil())

			tuf := tas.GetTuf(ctx, cli, namespace.Name, securesign.Name)()
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
