package v1alpha1

import (
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
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

			fetched.Spec.Config.OIDCIssuers[0] = OIDCIssuer{
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

			It("edit RouteSelectorLabel", func() {
				created := generateFulcioObject("fulcio-access-3")
				created.Spec.ExternalAccess.RouteSelectorLabels = map[string]string{"test": "fake", "foo": "bar"}
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.RouteSelectorLabels = map[string]string{"test": "test", "foo": "bar"}
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("RouteSelectorLabels can't be modified")))
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
			It("private key", func() {
				invalidObject := generateFulcioObject("private-key-invalid")
				invalidObject.Spec.Certificate.CARef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			It("config is not empty", func() {
				invalidObject := generateFulcioObject("config-invalid")
				invalidObject.Spec.Config.OIDCIssuers = []OIDCIssuer{}
				invalidObject.Spec.Config.MetaIssuers = []OIDCIssuer{}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("At least one of OIDCIssuers or MetaIssuers must be defined")))
			})

			It("only MetaIssuer is set", func() {
				validObject := generateFulcioObject("config-metaissuer")
				validObject.Spec.Config.OIDCIssuers = []OIDCIssuer{}
				validObject.Spec.Config.MetaIssuers = []OIDCIssuer{
					{
						ClientID: "client",
						Type:     "email",
					},
				}

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), fetched)).To(Succeed())
				Expect(fetched).To(Equal(validObject))
			})

			It("prefix with /", func() {
				validObject := generateFulcioObject("prefix-valid")
				validObject.Spec.Ctlog.Prefix = "logs/prefix"

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), fetched)).To(Succeed())
				Expect(fetched).To(Equal(validObject))
			})

			It("prefix with invalid chars", func() {
				invalidObject := generateFulcioObject("prefix-invalid")
				invalidObject.Spec.Ctlog.Prefix = "prefix.log"

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.ctlog.prefix in body should match")))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateFulcioObject("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateFulcioObject("replicas-positive")
					validObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateFulcioObject("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateFulcioObject("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
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
								OIDCIssuers: []OIDCIssuer{
									{
										ClientID:          "client",
										Type:              "email",
										IssuerURL:         "url",
										IssuerClaim:       "claim",
										ChallengeClaim:    "challenge",
										SPIFFETrustDomain: "SPIFFE",
										SubjectDomain:     "domain",
									},
									{
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
								CARef:                 &SecretKeySelector{Key: "key", LocalObjectReference: LocalObjectReference{Name: "name"}},
								PrivateKeyRef:         &SecretKeySelector{Key: "key", LocalObjectReference: LocalObjectReference{Name: "name"}},
								PrivateKeyPasswordRef: &SecretKeySelector{Key: "key", LocalObjectReference: LocalObjectReference{Name: "name"}},
							},
							Ctlog: CtlogService{
								Address: "ctlog.default.svc",
								Port:    ptr.To(int32(80)),
								Prefix:  "trusted-artifact-signer",
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
			PodRequirements: PodRequirements{
				Replicas: ptr.To(int32(1)),
			},
			Config: FulcioConfig{
				OIDCIssuers: []OIDCIssuer{
					{
						ClientID:  "client",
						Type:      "email",
						IssuerURL: "url",
						Issuer:    "url",
					},
				},
				MetaIssuers: []OIDCIssuer{
					{
						ClientID:  "client",
						Type:      "email",
						IssuerURL: "url",
						Issuer:    "url",
					},
					{
						ClientID: "client",
						Type:     "email",
						Issuer:   "url",
					},
				},
			},
			Certificate: FulcioCert{
				CommonName:       "hostname",
				OrganizationName: "organization",
			},
			Ctlog: CtlogService{
				Address: "",
				Port:    ptr.To(int32(80)),
				Prefix:  "trusted-artifact-signer",
			},
		},
	}
}
