package v1

import (
	"context"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	_ "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CTlog", func() {

	Context("CTlogSpec", func() {
		It("can be created", func() {
			created := generateMinimalCTlog("ctlog-create")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))
		})

		It("can be updated", func() {
			created := generateMinimalCTlog("ctlog-update")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			var id int64 = 1234567890123456789
			fetched.Spec.TreeID = &id
			Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
		})

		It("can be deleted", func() {
			created := generateMinimalCTlog("ctlog-delete")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), created)).ToNot(Succeed())
		})

		When("changing monitoring", func() {
			It("metrics enabled false->true", func() {
				created := generateMinimalCTlog("ctlog-monitoring-1")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &CTlog{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("metrics enabled true->false", func() {
				created := generateMinimalCTlog("ctlog-monitoring-2")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(true)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(false)
				Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

				fetched := &CTlog{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
				Expect(fetched).To(Equal(created))

				fetched.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				Expect(k8sClient.Update(context.Background(), fetched)).To(Succeed())
			})

			It("serviceMonitor requires metrics", func() {
				created := generateMinimalCTlog("ctlog-monitoring-3")
				created.Spec.Monitoring.Metrics.Enabled = ptr.To(false)
				created.Spec.Monitoring.ServiceMonitor.Enabled = ptr.To(true)
				Expect(k8sClient.Create(context.Background(), created)).
					To(MatchError(ContainSubstring("ServiceMonitor requires metrics to be enabled")))
			})
		})

		It("default constants are correct", func() {
			created := generateMinimalCTlog("ctlog-literals")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched := &CTlog{}
			Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(created), fetched)).To(Succeed())
			Expect(fetched.Spec.MaxCertChainSize).To(Equal(ptr.To(int64(153600))))
			Expect(fetched.Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(fetched.Spec.Monitoring.Metrics.Enabled).To(Equal(ptr.To(true)))
			Expect(fetched.Spec.Monitoring.ServiceMonitor.Enabled).To(Equal(ptr.To(false)))
			Expect(fetched.Spec.Monitoring.TLog.Enabled).To(Equal(ptr.To(false)))
			Expect(fetched.Spec.Monitoring.TLog.Interval).To(Equal(&metav1.Duration{Duration: 10 * time.Minute}))
		})

		Context("is validated", func() {
			It("publicKeyRef requires privateKeyRef", func() {
				invalidObject := generateMinimalCTlog("public-key-invalid")
				invalidObject.Spec.Signer.File = &CTlogFile{
					PublicKeyRef: &SecretKeySelector{
						Key:                  "key",
						LocalObjectReference: LocalObjectReference{Name: "name"},
					},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), invalidObject)).
					To(MatchError(ContainSubstring("privateKeyRef cannot be empty")))
			})

			When("replicas", func() {
				It("nil", func() {
					validObject := generateMinimalCTlog("replicas-nil")
					validObject.Spec.Replicas = nil
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("positive", func() {
					validObject := generateMinimalCTlog("replicas-positive")
					validObject.Spec.Replicas = ptr.To(int32(math.MaxInt32))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})

				It("negative", func() {
					invalidObject := generateMinimalCTlog("replicas-negative")
					invalidObject.Spec.Replicas = ptr.To(int32(-1))
					Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), invalidObject))).To(BeTrue())
					Expect(k8sClient.Create(context.Background(), invalidObject)).
						To(MatchError(ContainSubstring("spec.replicas in body should be greater than or equal to 0")))
				})

				It("zero", func() {
					validObject := generateMinimalCTlog("replicas-zero")
					validObject.Spec.Replicas = ptr.To(int32(0))
					Expect(k8sClient.Create(context.Background(), validObject)).To(Succeed())
				})
			})
		})

		Context("CR is fully populated", func() {
			It("outputs the CR", func() {
				tree := int64(1269875)
				ctlogInstance := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-full-manifest",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						Signer: CTlogSigner{
							Type: "file",
							File: &CTlogFile{
								PublicKeyRef: &SecretKeySelector{
									Key: "key",
									LocalObjectReference: LocalObjectReference{
										Name: "name",
									},
								},
								PrivateKeyRef: &SecretKeySelector{
									Key: "key",
									LocalObjectReference: LocalObjectReference{
										Name: "name",
									},
								},
							},
						},
						RootCertificates: []SecretKeySelector{
							{
								Key: "key",
								LocalObjectReference: LocalObjectReference{
									Name: "name",
								},
							},
						},
						Trillian: ServiceReference{},
					},
				}

				Expect(k8sClient.Create(context.Background(), &ctlogInstance)).To(Succeed())
				fetchedCTlog := &CTlog{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&ctlogInstance), fetchedCTlog)).To(Succeed())
				Expect(fetchedCTlog.Spec).To(Equal(ctlogInstance.Spec))
			})
		})

		Context("PKCS#11 Sharding", func() {
			It("supports PKCS#11 shard configuration", func() {
				tree := int64(1000)
				ctlogInstance := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-pkcs11-shard",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						Signer: CTlogSigner{
							Type: "file",
							File: &CTlogFile{},
						},
						Sharding: []CTlogLogRange{
							{
								TreeID:     10000,
								Type:       "pkcs11",
								Prefix:     "shard-hsm",
								ModulePath: "/usr/lib/softhsm/libsofthsm2.so",
								TokenLabel: "test-token",
								PinSecretRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "hsm-pin"},
									Key:                  "pin",
								},
								PublicKeyRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "shard-keys"},
									Key:                  "public-key.pem",
								},
							},
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &ctlogInstance)).To(Succeed())
				fetchedCTlog := &CTlog{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&ctlogInstance), fetchedCTlog)).To(Succeed())
				Expect(fetchedCTlog.Spec.Sharding).To(HaveLen(1))
				Expect(fetchedCTlog.Spec.Sharding[0].Type).To(Equal("pkcs11"))
				Expect(fetchedCTlog.Spec.Sharding[0].ModulePath).To(Equal("/usr/lib/softhsm/libsofthsm2.so"))
				Expect(fetchedCTlog.Spec.Sharding[0].TokenLabel).To(Equal("test-token"))
			})

			It("supports mixed file and PKCS#11 shards", func() {
				tree := int64(2000)
				ctlogInstance := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-mixed-shards",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						Signer: CTlogSigner{
							Type: "file",
							File: &CTlogFile{},
						},
						Sharding: []CTlogLogRange{
							{
								TreeID: 20000,
								Type:   "file",
								Prefix: "shard-file",
								PrivateKeyRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "keys"},
									Key:                  "key.pem",
								},
							},
							{
								TreeID:     20001,
								Type:       "pkcs11",
								Prefix:     "shard-hsm",
								ModulePath: "/usr/lib/softhsm/libsofthsm2.so",
								TokenLabel: "test-token",
								PinSecretRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "hsm-pin"},
									Key:                  "pin",
								},
								PublicKeyRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "shard-keys"},
									Key:                  "public-key.pem",
								},
							},
						},
					},
				}

				Expect(k8sClient.Create(context.Background(), &ctlogInstance)).To(Succeed())
				fetchedCTlog := &CTlog{}
				Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(&ctlogInstance), fetchedCTlog)).To(Succeed())
				Expect(fetchedCTlog.Spec.Sharding).To(HaveLen(2))
				Expect(fetchedCTlog.Spec.Sharding[0].Type).To(Equal("file"))
				Expect(fetchedCTlog.Spec.Sharding[1].Type).To(Equal("pkcs11"))
			})

			It("PKCS#11 shard requires modulePath", func() {
				tree := int64(3000)
				invalidObject := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-pkcs11-missing-module",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						Signer: CTlogSigner{
							Type: "file",
							File: &CTlogFile{},
						},
						Sharding: []CTlogLogRange{
							{
								TreeID:     30000,
								Type:       "pkcs11",
								Prefix:     "shard-hsm",
								TokenLabel: "test-token",
								PinSecretRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "hsm-pin"},
									Key:                  "pin",
								},
								PublicKeyRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "shard-keys"},
									Key:                  "public-key.pem",
								},
							},
						},
					},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), &invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), &invalidObject)).
					To(MatchError(ContainSubstring("modulePath is required for pkcs11-type shards")))
			})

			It("PKCS#11 shard requires tokenLabel", func() {
				tree := int64(3001)
				invalidObject := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-pkcs11-missing-token",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						Signer: CTlogSigner{
							Type: "file",
							File: &CTlogFile{},
						},
						Sharding: []CTlogLogRange{
							{
								TreeID:     30001,
								Type:       "pkcs11",
								Prefix:     "shard-hsm",
								ModulePath: "/usr/lib/softhsm/libsofthsm2.so",
								PinSecretRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "hsm-pin"},
									Key:                  "pin",
								},
								PublicKeyRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "shard-keys"},
									Key:                  "public-key.pem",
								},
							},
						},
					},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), &invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), &invalidObject)).
					To(MatchError(ContainSubstring("tokenLabel is required for pkcs11-type shards")))
			})

			It("PKCS#11 shard requires pinSecretRef", func() {
				tree := int64(3002)
				invalidObject := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-pkcs11-missing-pin",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						Signer: CTlogSigner{
							Type: "file",
							File: &CTlogFile{},
						},
						Sharding: []CTlogLogRange{
							{
								TreeID:     30002,
								Type:       "pkcs11",
								Prefix:     "shard-hsm",
								ModulePath: "/usr/lib/softhsm/libsofthsm2.so",
								TokenLabel: "test-token",
								PublicKeyRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "shard-keys"},
									Key:                  "public-key.pem",
								},
							},
						},
					},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), &invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), &invalidObject)).
					To(MatchError(ContainSubstring("pinSecretRef is required for pkcs11-type shards")))
			})

			It("PKCS#11 shard requires publicKeyRef", func() {
				tree := int64(3003)
				invalidObject := CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ctlog-pkcs11-missing-pubkey",
						Namespace: "default",
					},
					Spec: CTlogSpec{
						TreeID: &tree,
						Signer: CTlogSigner{
							Type: "file",
							File: &CTlogFile{},
						},
						Sharding: []CTlogLogRange{
							{
								TreeID:     30003,
								Type:       "pkcs11",
								Prefix:     "shard-hsm",
								ModulePath: "/usr/lib/softhsm/libsofthsm2.so",
								TokenLabel: "test-token",
								PinSecretRef: &SecretKeySelector{
									LocalObjectReference: LocalObjectReference{Name: "hsm-pin"},
									Key:                  "pin",
								},
							},
						},
					},
				}

				Expect(apierrors.IsInvalid(k8sClient.Create(context.Background(), &invalidObject))).To(BeTrue())
				Expect(k8sClient.Create(context.Background(), &invalidObject)).
					To(MatchError(ContainSubstring("publicKeyRef is required for pkcs11-type shards")))
			})
		})
	})
})

func generateMinimalCTlog(name string) *CTlog {
	return &CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: CTlogSpec{
			Signer: CTlogSigner{Type: "file"},
		},
	}
}
