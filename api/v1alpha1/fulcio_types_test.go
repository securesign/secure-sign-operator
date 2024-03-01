package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	_ "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Fulcio", func() {

	Context("FulcioSpec", func() {
		It("can be created", func() {
			created := generateFulcioObject("fulcio-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateFulcioObject("fulcio-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Config.OIDCIssuers["test"] = OIDCIssuer{
				Type:     "email",
				ClientID: "client",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateFulcioObject("fulcio-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateFulcioObject("fulcio-access-1")
				created.Spec.ExternalAccess.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateFulcioObject("fulcio-access-2")
				created.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		When("changing monitoring", func() {
			It("enabled false->true", func() {
				created := generateFulcioObject("fulcio-monitoring-1")
				created.Spec.Monitoring.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateFulcioObject("fulcio-monitoring-2")
				created.Spec.Monitoring.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		Context("is validated", func() {
			It("commonName", func() {
				invalidObject := generateFulcioObject("commonname-invalid")
				invalidObject.Spec.Certificate.CommonName = ""

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("commonName cannot be empty")))
			})

			It("private key", func() {
				invalidObject := generateFulcioObject("private-key-invalid")
				invalidObject.Spec.Certificate.CARef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: corev1.LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			It("config is not empty", func() {
				invalidObject := generateFulcioObject("config-invalid")
				invalidObject.Spec.Config.OIDCIssuers = make(map[string]OIDCIssuer)

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("in body should have at least 1 properties")))
			})
		})

		Context("Default settings", func() {
			var (
				fulcioInstance Fulcio
			)

			When("CR spec is empty", func() {
				It("creates CR with defaults", func() {
					fulcioInstance = Fulcio{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "fulcio-defaults",
							Namespace: "default",
						},
					}

					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), &fulcioInstance))).To(BeTrue())
				})
			})

			When("CR is fully populated", func() {
				It("outputs the CR", func() {
					fulcioInstance = Fulcio{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "fulcio-full-manifest",
							Namespace: "default",
						},
						Spec: FulcioSpec{
							Monitoring: MonitoringConfig{
								Enabled: true,
							},
							ExternalAccess: ExternalAccess{
								Enabled: true,
								Host:    "hostname",
							},
							Config: FulcioConfig{
								OIDCIssuers: map[string]OIDCIssuer{
									"oidc": {
										ClientID:          "client",
										Type:              "email",
										IssuerURL:         "url",
										IssuerClaim:       "claim",
										ChallengeClaim:    "challange",
										SPIFFETrustDomain: "SPIFFE",
										SubjectDomain:     "domain",
									},
									"oidc2": {
										ClientID:          "clien2",
										Type:              "email2",
										IssuerURL:         "url2",
										IssuerClaim:       "claim2",
										ChallengeClaim:    "challang2e",
										SPIFFETrustDomain: "SPIFFE2",
										SubjectDomain:     "domain2",
									},
								},
							},
							Certificate: FulcioCert{
								CommonName:            "CommonName",
								OrganizationName:      "OrganizationName",
								OrganizationEmail:     "OrganizationEmail",
								CARef:                 &SecretKeySelector{Key: "key", LocalObjectReference: corev1.LocalObjectReference{Name: "name"}},
								PrivateKeyRef:         &SecretKeySelector{Key: "key", LocalObjectReference: corev1.LocalObjectReference{Name: "name"}},
								PrivateKeyPasswordRef: &SecretKeySelector{Key: "key", LocalObjectReference: corev1.LocalObjectReference{Name: "name"}},
							},
						},
					}

					Expect(k8sClient.Create(context.Background(), &fulcioInstance)).To(Succeed())
					fetchedFulcio := &Fulcio{}
					Expect(k8sClient.Get(context.Background(), getKey(&fulcioInstance), fetchedFulcio)).To(Succeed())
					Expect(fetchedFulcio.Spec).To(Equal(fulcioInstance.Spec))
				})
			})
		})
	})
})

func generateFulcioObject(name string) *Fulcio {
	return &Fulcio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: FulcioSpec{
			Config: FulcioConfig{
				OIDCIssuers: map[string]OIDCIssuer{
					"oidc": {
						ClientID:  "client",
						Type:      "email",
						IssuerURL: "url",
					},
				},
			},
			Certificate: FulcioCert{
				CommonName: "hostname",
			},
		},
	}
}
