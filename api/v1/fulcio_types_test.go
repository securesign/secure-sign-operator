package v1

import (
	"context"
	"math"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Fulcio", func() {

	Context("FulcioSpec", func() {
		It("can be created", func() {
			created := generateMinimalFulcio("fulcio-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalFulcio("fulcio-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			fetched.Spec.Config.OIDCIssuers[0] = OIDCIssuer{
				Issuer:   "https://updated.example.com",
				Type:     "email",
				ClientID: "client",
			}
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalFulcio("fulcio-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateMinimalFulcio("fulcio-access-1")
				created.Spec.ExternalAccess.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateMinimalFulcio("fulcio-access-2")
				created.Spec.ExternalAccess.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = ptr.To(false)
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})

			It("edit RouteSelectorLabel", func() {
				created := generateMinimalFulcio("fulcio-access-3")
				created.Spec.ExternalAccess.RouteSelectorLabels = map[string]string{"test": "fake", "foo": "bar"}
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.RouteSelectorLabels = map[string]string{"test": "test", "foo": "bar"}
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("RouteSelectorLabels can't be modified")))
			})
		})

		When("changing monitoring", func() {
			It("enabled false->true", func() {
				created := generateMinimalFulcio("fulcio-monitoring-1")
				created.Spec.Monitoring.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateMinimalFulcio("fulcio-monitoring-2")
				created.Spec.Monitoring.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Enabled = ptr.To(false)
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		Context("is validated", func() {
			It("private key", func() {
				invalidObject := generateMinimalFulcio("private-key-invalid")
				invalidObject.Spec.Certificate.CARef = &SecretKeySelector{
					Key:                  "key",
					LocalObjectReference: LocalObjectReference{Name: "name"},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			It("config is not empty", func() {
				invalidObject := generateMinimalFulcio("config-invalid")
				invalidObject.Spec.Config.OIDCIssuers = []OIDCIssuer{}
				invalidObject.Spec.Config.MetaIssuers = []OIDCIssuer{}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("At least one of oidcIssuers or metaIssuers must be defined")))
			})

			It("CIIssuerMetadata is set", func() {
				validObject := generateMinimalFulcio("config-ci-issuer-metadata")
				addCIIssuerMetadata(validObject)

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), fetched)).To(Succeed())
				Expect(fetched).To(Equal(validObject))
			})

			It("only MetaIssuer is set", func() {
				validObject := generateMinimalFulcio("config-metaissuer")
				validObject.Spec.Config.OIDCIssuers = []OIDCIssuer{}
				validObject.Spec.Config.MetaIssuers = []OIDCIssuer{
					{
						Issuer:   "https://meta.example.com",
						ClientID: "client",
						Type:     "email",
					},
				}

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), fetched)).To(Succeed())
				Expect(fetched).To(Equal(validObject))
			})

			It("prefix with /", func() {
				validObject := generateMinimalFulcio("prefix-valid")
				validObject.Spec.Ctlog.Prefix = "logs/prefix"

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				fetched := &Fulcio{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), fetched)).To(Succeed())
				Expect(fetched).To(Equal(validObject))
			})

			It("prefix with invalid chars", func() {
				invalidObject := generateMinimalFulcio("prefix-invalid")
				invalidObject.Spec.Ctlog.Prefix = "prefix.log"

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.ctlog.prefix in body should match")))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateMinimalFulcio("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateMinimalFulcio("replicas-positive")
					validObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateMinimalFulcio("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateMinimalFulcio("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})

		It("default constants are correct", func() {
			created := generateMinimalFulcio("fulcio-literals")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Fulcio{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.Ctlog.Prefix).To(Equal("trusted-artifact-signer"))
			Expect(fetched.Spec.Monitoring.Enabled).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.ExternalAccess.Enabled).To(Equal(ptr.To(false)))
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				fulcioInstance := Fulcio{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fulcio-full-manifest",
						Namespace: "default",
					},
					Spec: FulcioSpec{
						Monitoring: MonitoringConfig{
							Enabled: ptr.To(true),
						},
						ExternalAccess: ExternalAccess{
							Enabled: ptr.To(true),
							Host:    "hostname",
						},
						Config: FulcioConfig{
							OIDCIssuers: []OIDCIssuer{
								{
									Issuer:            "https://issuer1.example.com",
									ClientID:          "client",
									Type:              "email",
									IssuerURL:         "https://issuer1.example.com",
									IssuerClaim:       "claim",
									ChallengeClaim:    "challenge",
									SPIFFETrustDomain: "SPIFFE",
									SubjectDomain:     "domain",
								},
								{
									Issuer:            "https://issuer2.example.com",
									ClientID:          "client2",
									Type:              "username",
									IssuerURL:         "https://issuer2.example.com",
									IssuerClaim:       "claim2",
									ChallengeClaim:    "challenge2",
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
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&fulcioInstance), fetchedFulcio)).To(Succeed())
				Expect(fetchedFulcio.Spec).To(Equal(fulcioInstance.Spec))
			})
		})
	})
})

func generateMinimalFulcio(name string) *Fulcio {
	return &Fulcio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: FulcioSpec{
			Config: FulcioConfig{
				OIDCIssuers: []OIDCIssuer{
					{
						ClientID:   "client",
						Type:       "email",
						IssuerURL:  "https://issuer1.example.com",
						Issuer:     "https://issuer1.example.com",
						CIProvider: "foo",
					},
					{
						ClientID:   "ci-client",
						Type:       "ci-provider",
						CIProvider: "foo",
						IssuerURL:  "https://issuer2.example.com",
						Issuer:     "https://issuer2.example.com",
					},
				},
				MetaIssuers: []OIDCIssuer{
					{
						ClientID:  "client",
						Type:      "email",
						IssuerURL: "https://meta1.example.com",
						Issuer:    "https://meta1.example.com",
					},
					{
						ClientID: "client",
						Type:     "email",
						Issuer:   "https://meta2.example.com",
					},
				},
			},
			Certificate: FulcioCert{
				CommonName:       "hostname",
				OrganizationName: "organization",
			},
		},
	}
}

func addCIIssuerMetadata(config *Fulcio) *Fulcio {
	config.Spec.Config.CIIssuerMetadata = []CIIssuerMetadata{
		{
			IssuerName:                     "gitlab-ci",
			DefaultTemplateValues:          map[string]string{"url": "https://gitlab.com"},
			SubjectAlternativeNameTemplate: "https://{{ .ci_config_ref_uri }}",
			ExtensionTemplates: Extensions{
				BuildSignerURI:                      "https://{{ .ci_config_ref_uri }}",
				BuildSignerDigest:                   "ci_config_sha",
				RunnerEnvironment:                   "runner_environment",
				SourceRepositoryURI:                 "{{ .url }}/{{ .project_path }}",
				SourceRepositoryDigest:              "sha",
				SourceRepositoryRef:                 "refs/{{if eq .ref_type \"branch\"}}heads/{{ else }}tags/{{end}}{{ .ref }}",
				SourceRepositoryIdentifier:          "project_id",
				SourceRepositoryOwnerURI:            "{{ .url }}/{{ .namespace_path }}",
				SourceRepositoryOwnerIdentifier:     "namespace_id",
				BuildConfigURI:                      "https://{{ .ci_config_ref_uri }}",
				BuildConfigDigest:                   "ci_config_sha",
				BuildTrigger:                        "pipeline_source",
				RunInvocationURI:                    "{{ .url }}/{{ .project_path }}/-/jobs/{{ .job_id }}",
				SourceRepositoryVisibilityAtSigning: "project_visibility",
			},
		},
	}
	return config
}
