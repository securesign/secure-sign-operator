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
)

var _ = Describe("Rekor", func() {

	Context("RekorSpec", func() {
		It("can be created", func() {
			created := generateRekorObject("rekor-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Rekor{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateRekorObject("rekor-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &Rekor{}
			Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			var id int64 = 1234567890123456789
			fetched.Spec.TreeID = &id
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateRekorObject("rekor-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), getKey(created), created)).ToNot(Succeed())
		})

		When("changing external access setting", func() {
			It("enabled false->true", func() {
				created := generateRekorObject("rekor-access-1")
				created.Spec.ExternalAccess.Enabled = false
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("enabled true->false", func() {
				created := generateRekorObject("rekor-access-2")
				created.Spec.ExternalAccess.Enabled = true
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.ExternalAccess.Enabled = false
				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), fetched))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), fetched)).
					To(MatchError(ContainSubstring("Feature cannot be disabled")))
			})
		})

		Context("is validated", func() {
			It("cron syntax", func() {
				invalidObject := generateRekorObject("backfill-schedule")
				invalidObject.Spec.BackFillRedis.Schedule = "@invalid"

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("schedule in body should match")))
			})

			It("immutable pvc retain", func() {
				validObject := generateRekorObject("immutable-retain")
				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())

				invalidObject := &Rekor{}
				Expect(k8sClient.Get(context.Background(), getKey(validObject), invalidObject)).To(Succeed())
				invalidObject.Spec.Attestations.Pvc.Retain = ptr.To(false)

				Expect(apierrors.IsInvalid(k8sClient.Update(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Update(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("Field is immutable")))
			})

			It("checking pvc name", func() {
				invalidObject := generateRekorObject("rekor3")
				invalidObject.Spec.Attestations.Pvc.Name = "-invalid-name!"
				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("spec.attestations.pvc.name in body should match")))
			})
		})

		Context("sharding", func() {
			It("require treeId", func() {
				invalidObject := generateRekorObject("sharding-treeid")
				invalidObject.Spec.Sharding = []RekorLogRange{
					{},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("treeID in body should be greater than or equal to 1")))
			})

			It("duplicate trees", func() {
				invalidObject := generateRekorObject("sharding-duplicate")
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
					validObject := generateRekorObject("rekor-kms-valid-secret")
					validObject.Spec.Signer.KMS = "secret"
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("should allow 'awskms://' URI", func() {
					validObject := generateRekorObject("rekor-kms-valid-aws")
					validObject.Spec.Signer.KMS = "awskms://key/arn"
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})

			When("using invalid KMS values", func() {
				It("should reject a random string", func() {
					invalidObject := generateRekorObject("rekor-kms-invalid-random")
					invalidObject.Spec.Signer.KMS = "unsupported"

					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("KMS must be '', 'secret', 'memory', or a valid URI")))
				})
			})
		})

		Context("PVC access mode validation with replicas", func() {
			It("should reject replicas > 1 with file:// URL and ReadWriteOnce", func() {
				invalidObject := generateRekorObject("rekor-ha-rwo-invalid")
				invalidObject.Spec.Replicas = ptr.To(int32(2))
				invalidObject.Spec.Attestations.Enabled = ptr.To(true)
				invalidObject.Spec.Attestations.Url = "file:///var/run/attestations?no_tmp_dir=true"
				invalidObject.Spec.Attestations.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteOnce"}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("PVC accessModes must contain 'ReadWriteMany' for replicas greater than 1")))
			})

			It("should allow replicas > 1 with file:// URL and ReadWriteMany", func() {
				validObject := generateRekorObject("rekor-ha-rwx-valid")
				validObject.Spec.Replicas = ptr.To(int32(2))
				validObject.Spec.Attestations.Enabled = ptr.To(true)
				validObject.Spec.Attestations.Url = "file:///var/run/attestations?no_tmp_dir=true"
				validObject.Spec.Attestations.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteMany"}

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("should allow replicas > 1 with non-file:// URL and ReadWriteOnce", func() {
				validObject := generateRekorObject("rekor-ha-s3-rwo-valid")
				validObject.Spec.Replicas = ptr.To(int32(2))
				validObject.Spec.Attestations.Enabled = ptr.To(true)
				validObject.Spec.Attestations.Url = "s3://my-bucket?region=us-west-1"
				validObject.Spec.Attestations.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteOnce"}

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("should allow single replica with file:// URL and ReadWriteOnce", func() {
				validObject := generateRekorObject("rekor-single-rwo-valid")
				validObject.Spec.Replicas = ptr.To(int32(1))
				validObject.Spec.Attestations.Enabled = ptr.To(true)
				validObject.Spec.Attestations.Url = "file:///var/run/attestations?no_tmp_dir=true"
				validObject.Spec.Attestations.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteOnce"}

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})

			It("should allow replicas > 1 when attestations are disabled", func() {
				validObject := generateRekorObject("rekor-ha-attestations-disabled")
				validObject.Spec.Replicas = ptr.To(int32(2))
				validObject.Spec.Attestations.Enabled = ptr.To(false)
				validObject.Spec.Attestations.Pvc.AccessModes = []PersistentVolumeAccessMode{"ReadWriteOnce"}

				Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
			})
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				storage := k8sresource.MustParse("987Gi")
				tree := int64(1269875)
				port := int32(8091)

				rekorInstance := Rekor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rekor-full-manifest",
						Namespace: "default",
					},
					Spec: RekorSpec{
						Monitoring: MonitoringWithTLogConfig{
							MonitoringConfig: MonitoringConfig{
								Enabled: true,
							},
							TLog: TlogMonitoring{
								Enabled: true,
							},
						},
						ExternalAccess: ExternalAccess{
							Enabled: true,
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
						Trillian: TrillianService{
							Address: "trillian-system.default.svc",
							Port:    &port,
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
				Expect(k8sClient.Get(context.Background(), getKey(&rekorInstance), fetchedRekor)).To(Succeed())
				Expect(fetchedRekor.Spec).To(Equal(rekorInstance.Spec))
			})
		})
	})
})

func generateRekorObject(name string) *Rekor {
	storage := k8sresource.MustParse("5Gi")
	maxSize := k8sresource.MustParse("100Ki")
	return &Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: RekorSpec{
			BackFillRedis: BackFillRedis{
				Enabled:  ptr.To(true),
				Schedule: "0 0 * * *",
			},
			Signer: RekorSigner{
				KMS: "secret",
			},
			Attestations: RekorAttestations{
				Enabled: ptr.To(true),
				Url:     "file:///var/run/attestations?no_tmp_dir=true",
				MaxSize: &maxSize,
				Pvc: Pvc{
					Retain: ptr.To(true),
					Size:   &storage,
					AccessModes: []PersistentVolumeAccessMode{
						"ReadWriteOnce",
					},
				},
			},
			Trillian: TrillianService{
				Port: ptr.To(int32(8091)),
			},
			MaxRequestBodySize: ptr.To(int64(10485760)),
		},
	}
}
