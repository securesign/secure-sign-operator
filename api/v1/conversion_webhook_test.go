/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Conversion webhook", func() {
	const testNs = "default"

	Context("CTlog", func() {
		It("should create v1 and read as v1alpha1", func() {
			v1obj := &CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog-v1-test",
					Namespace: testNs,
				},
				Spec: CTlogSpec{
					PodRequirements: PodRequirements{Replicas: ptr.To[int32](1)},
					TreeID:          ptr.To[int64](12345),
				},
			}
			Expect(k8sClient.Create(ctx, v1obj)).To(Succeed())

			v1alpha1obj := &v1alpha1.CTlog{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ctlog-v1-test", Namespace: testNs}, v1alpha1obj)).To(Succeed())
			Expect(v1alpha1obj.Spec.TreeID).ToNot(BeNil())
			Expect(*v1alpha1obj.Spec.TreeID).To(Equal(int64(12345)))
		})

		It("should create v1alpha1 and read as v1", func() {
			v1alpha1obj := &v1alpha1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog-v1alpha1-test",
					Namespace: testNs,
				},
				Spec: v1alpha1.CTlogSpec{
					TreeID: ptr.To[int64](67890),
				},
			}
			Expect(k8sClient.Create(ctx, v1alpha1obj)).To(Succeed())

			v1obj := &CTlog{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ctlog-v1alpha1-test", Namespace: testNs}, v1obj)).To(Succeed())
			Expect(v1obj.Spec.TreeID).ToNot(BeNil())
			Expect(*v1obj.Spec.TreeID).To(Equal(int64(67890)))
		})
	})

	Context("Rekor", func() {
		It("should round-trip v1 → v1alpha1 → v1 through the API server", func() {
			v1obj := &Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rekor-roundtrip",
					Namespace: testNs,
				},
				Spec: RekorSpec{
					TreeID: ptr.To[int64](11111),
					Signer: RekorSigner{KMS: "secret"},
					Attestations: RekorAttestations{
						Enabled: ptr.To(true),
						Url:     "file:///var/run/attestations?no_tmp_dir=true",
					},
				},
			}
			Expect(k8sClient.Create(ctx, v1obj)).To(Succeed())

			v1alpha1obj := &v1alpha1.Rekor{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rekor-roundtrip", Namespace: testNs}, v1alpha1obj)).To(Succeed())
			Expect(v1alpha1obj.Spec.Signer.KMS).To(Equal("secret"))
			Expect(v1alpha1obj.Spec.Attestations.Enabled).ToNot(BeNil())
			Expect(*v1alpha1obj.Spec.Attestations.Enabled).To(BeTrue())

			v1ReadBack := &Rekor{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rekor-roundtrip", Namespace: testNs}, v1ReadBack)).To(Succeed())
			Expect(equality.Semantic.DeepEqual(v1obj.Spec, v1ReadBack.Spec)).To(BeTrue(),
				"v1 → API server → v1 spec should be identical")
		})
	})

	Context("Fulcio", func() {
		It("should preserve OIDC config across versions", func() {
			v1obj := &Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fulcio-oidc-test",
					Namespace: testNs,
				},
				Spec: FulcioSpec{
					Config: FulcioConfig{
						OIDCIssuers: []OIDCIssuer{
							{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
						},
					},
					Certificate: FulcioCert{OrganizationName: "Red Hat"},
				},
			}
			Expect(k8sClient.Create(ctx, v1obj)).To(Succeed())

			v1alpha1obj := &v1alpha1.Fulcio{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "fulcio-oidc-test", Namespace: testNs}, v1alpha1obj)).To(Succeed())
			Expect(v1alpha1obj.Spec.Config.OIDCIssuers).To(HaveLen(1))
			Expect(v1alpha1obj.Spec.Config.OIDCIssuers[0].Issuer).To(Equal("https://accounts.google.com"))
			Expect(v1alpha1obj.Spec.Certificate.OrganizationName).To(Equal("Red Hat"))
		})
	})

	Context("Trillian", func() {
		It("should convert database config between versions", func() {
			v1obj := &Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "trillian-convert-test",
					Namespace: testNs,
				},
				Spec: TrillianSpec{
					Db: TrillianDB{
						Create:   ptr.To(true),
						Provider: "mysql",
					},
				},
			}
			Expect(k8sClient.Create(ctx, v1obj)).To(Succeed())

			v1alpha1obj := &v1alpha1.Trillian{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "trillian-convert-test", Namespace: testNs}, v1alpha1obj)).To(Succeed())
			Expect(v1alpha1obj.Spec.Db.Create).ToNot(BeNil())
			Expect(*v1alpha1obj.Spec.Db.Create).To(BeTrue())
			Expect(v1alpha1obj.Spec.Db.Provider).To(Equal("mysql"))
		})
	})

	Context("Tuf", func() {
		It("should preserve keys across versions", func() {
			v1obj := &Tuf{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tuf-keys-test",
					Namespace: testNs,
				},
				Spec: TufSpec{
					Port: 80,
					Keys: []TufKey{
						{Name: "rekor.pub"},
						{Name: "ctfe.pub"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, v1obj)).To(Succeed())

			v1alpha1obj := &v1alpha1.Tuf{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "tuf-keys-test", Namespace: testNs}, v1alpha1obj)).To(Succeed())
			Expect(v1alpha1obj.Spec.Keys).To(HaveLen(2))
			Expect(v1alpha1obj.Spec.Port).To(Equal(int32(80)))
		})
	})

	Context("TimestampAuthority", func() {
		It("should preserve signer config across versions", func() {
			v1alpha1obj := &v1alpha1.TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tsa-signer-test",
					Namespace: testNs,
				},
				Spec: v1alpha1.TimestampAuthoritySpec{
					Signer: v1alpha1.TimestampAuthoritySigner{
						CertificateChain: v1alpha1.CertificateChain{
							RootCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName: "Test Org",
							},
							LeafCA: &v1alpha1.TsaCertificateAuthority{
								OrganizationName: "Test Org",
							},
							IntermediateCA: []*v1alpha1.TsaCertificateAuthority{
								{OrganizationName: "Test Org"},
							},
						},
					},
					NTPMonitoring: v1alpha1.NTPMonitoring{Enabled: true},
				},
			}
			Expect(k8sClient.Create(ctx, v1alpha1obj)).To(Succeed())

			v1obj := &TimestampAuthority{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "tsa-signer-test", Namespace: testNs}, v1obj)).To(Succeed())
			Expect(v1obj.Spec.Signer.CertificateChain.RootCA).ToNot(BeNil())
			Expect(v1obj.Spec.Signer.CertificateChain.RootCA.OrganizationName).To(Equal("Test Org"))
			Expect(v1obj.Spec.NTPMonitoring.Enabled).To(BeTrue())
		})
	})

	Context("Securesign", func() {
		It("should convert umbrella CR between versions", func() {
			v1obj := &Securesign{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "securesign-test",
					Namespace: testNs,
				},
				Spec: SecuresignSpec{
					Rekor: RekorSpec{
						Signer: RekorSigner{KMS: "secret"},
					},
					Fulcio: FulcioSpec{
						Config: FulcioConfig{
							OIDCIssuers: []OIDCIssuer{
								{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
							},
						},
						Certificate: FulcioCert{OrganizationName: "Red Hat"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, v1obj)).To(Succeed())

			v1alpha1obj := &v1alpha1.Securesign{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "securesign-test", Namespace: testNs}, v1alpha1obj)).To(Succeed())
			Expect(v1alpha1obj.Spec.Rekor.Signer.KMS).To(Equal("secret"))
			Expect(v1alpha1obj.Spec.Fulcio.Certificate.OrganizationName).To(Equal("Red Hat"))
		})
	})
})
