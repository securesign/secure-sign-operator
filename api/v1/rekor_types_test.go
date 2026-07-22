package v1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rekor", func() {

	Context("RekorSpec", func() {
		It("can be created", func() {
			created := generateMinimalRekor("rekor-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Rekor{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalRekor("rekor-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Rekor{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			var id int64 = 1234567890123456789
			fetched.Spec.TreeID = &id
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalRekor("rekor-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateMinimalRekor("rekor-access-1")
				created.Spec.Ingress.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Ingress.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateMinimalRekor("rekor-access-2")
				created.Spec.Ingress.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Ingress.Enabled = ptr.To(false)
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		When("changing monitoring", func() {
			It("metrics enabled false->true", func() {
				created := generateMinimalRekor("rekor-monitoring-1")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("metrics enabled true->false", func() {
				created := generateMinimalRekor("rekor-monitoring-2")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("serviceMonitor requires metrics", func() {
				created := generateMinimalRekor("rekor-monitoring-3")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).
					To(MatchError(ContainSubstring("ServiceMonitor requires metrics to be enabled")))
			})
		})

		It("default constants are correct", func() {
			created := generateMinimalRekor("rekor-literals")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Rekor{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec.MaxRequestBodySize).To(Equal(ptr.To(int64(10485760))))
			Expect(fetched.Spec.Trillian).To(Equal(ServiceReference{}))
			Expect(fetched.Spec.Signer.KMS).To(Equal("secret"))
			Expect(fetched.Spec.BackFillRedis.Schedule).To(Equal("0 0 * * *"))
			Expect(fetched.Spec.BackFillRedis.Enabled).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Attestations.Pvc.Size).To(Equal(ptr.To(k8sresource.MustParse("5Gi"))))
			Expect(fetched.Spec.Attestations.Pvc.Retain).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.Attestations.Url).To(Equal("file:///var/run/attestations?no_tmp_dir=true"))
			Expect(fetched.Spec.Monitoring.Metrics.Enabled).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Monitoring.ServiceMonitor.Enabled).To(Equal(ptr.To(false)))
			Expect(fetched.Spec.Monitoring.TLog.Enabled).To(Equal(ptr.To(false)))
		})

		Context("is validated", func() {
			It("cron syntax", func() {
				invalidObject := generateMinimalRekor("backfill-schedule")
				invalidObject.Spec.BackFillRedis.Schedule = "@invalid"

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("schedule in body should match")))
			})

			It("immutable pvc retain", func() {
				validObject := generateMinimalRekor("immutable-retain")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Rekor{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Attestations.Pvc.Retain = ptr.To(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			It("checking pvc name", func() {
				invalidObject := generateMinimalRekor("rekor3")
				invalidObject.Spec.Attestations.Pvc.Name = "-invalid-name!"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.attestations.pvc.name in body should match")))
			})
		})

		Context("sharding", func() {
			It("require treeId", func() {
				invalidObject := generateMinimalRekor("sharding-treeid")
				invalidObject.Spec.Sharding = []RekorLogRange{
					{},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("treeID in body should be greater than or equal to 1")))
			})

			It("duplicate trees", func() {
				invalidObject := generateMinimalRekor("sharding-duplicate")
				invalidObject.Spec.Sharding = []RekorLogRange{
					{
						TreeID: 123,
					},
					{
						TreeID:     123,
						TreeLength: 1,
					},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Duplicate value")))
			})
		})

		Context("signer validation", func() {
			When("using valid KMS values", func() {
				It("should allow 'secret'", func() {
					validObject := generateMinimalRekor("rekor-kms-valid-secret")
					validObject.Spec.Signer.KMS = "secret"
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("should allow 'awskms://' URI", func() {
					validObject := generateMinimalRekor("rekor-kms-valid-aws")
					validObject.Spec.Signer.KMS = "awskms://key/arn"
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})

			When("using invalid KMS values", func() {
				It("should reject a random string", func() {
					invalidObject := generateMinimalRekor("rekor-kms-invalid-random")
					invalidObject.Spec.Signer.KMS = "unsupported"

					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("KMS must be 'secret', 'memory', or a valid URI")))
				})
			})
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				storage := k8sresource.MustParse("987Gi")
				tree := int64(1269875)

				rekorInstance := Rekor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rekor-full-manifest",
						Namespace: "default",
					},
					Spec: RekorSpec{
						Monitoring: MonitoringWithTLogConfig{
							MonitoringConfig: MonitoringConfig{
								Metrics:        MetricsConfig{Enabled: ptr.To(true)},
								ServiceMonitor: ServiceMonitorConfig{Enabled: ptr.To(true)},
							},
							TLog: TlogMonitoring{
								Enabled: ptr.To(true),
							},
						},
						Ingress: Ingress{
							Enabled: ptr.To(true),
							Host:    "hostname",
						},
						RekorSearchUI: RekorSearchUI{
							Enabled: ptr.To(true),
						},
						BackFillRedis: BackFillRedis{
							Enabled:  ptr.To(true),
							Schedule: "* */2 * * 0-3",
						},
						TreeID: &tree,
						Attestations: RekorAttestations{
							Pvc: Pvc{
								Name:         "name",
								Size:         &storage,
								StorageClass: "name",
								Retain:       ptr.To(true),
							},
						},
						Signer: RekorSigner{
							KMS: "secret",
							KeyRef: &SecretKeySelector{
								LocalObjectReference: LocalObjectReference{
									Name: "secret",
								},
								Key: "key",
							},
							PasswordRef: &SecretKeySelector{
								LocalObjectReference: LocalObjectReference{
									Name: "secret",
								},
								Key: "key",
							},
						},
						Trillian: ServiceReference{
							URL: "trillian-system.default.svc:8091",
						},
						Sharding: []RekorLogRange{
							{
								TreeID:           123456789,
								TreeLength:       1,
								EncodedPublicKey: "LS0tLS1CRUdJTiBQVUJMSUMgS0VZLS0tLS0KTUZrd0V3WUhLb1pJemowQ0FRWUlLb1pJemowREFRY0RRZ0FFWkZ0Nk5FcU14YWVVNzZsbmxZekZVTmpGUUdIcQpORjQ2QlBDVGxQL0ZnZk1aak42MDhjRFhmM0xNNWhUYnZOeUNFYWJFKzRNYk9jRU1YaERRVWxZRnZBPT0KLS0tLS1FTkQgUFVCTElDIEtFWS0tLS0tCg==",
							},
						},
						SearchIndex: SearchIndex{
							Create: ptr.To(true),
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &rekorInstance)).To(Succeed())
				fetchedRekor := &Rekor{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&rekorInstance), fetchedRekor)).To(Succeed())
				Expect(fetchedRekor.Spec).To(Equal(rekorInstance.Spec))
			})
		})
	})
})

func generateMinimalRekor(name string) *Rekor {
	return &Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
}
