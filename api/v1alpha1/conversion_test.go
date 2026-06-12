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

package v1alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	v1 "github.com/securesign/operator/api/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// conversionTestCase defines a single conversion test case.
type conversionTestCase struct {
	name  string
	hub   func() *v1.Securesign
	spoke func() *Securesign
}

func TestSecuresignConversionUnit(t *testing.T) {
	tests := []conversionTestCase{
		{
			name: "empty spec round-trips",
			hub: func() *v1.Securesign {
				return &v1.Securesign{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				}
			},
			spoke: func() *Securesign {
				return &Securesign{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
				}
			},
		},
		{
			name: "full spec with all components",
			hub: func() *v1.Securesign {
				return &v1.Securesign{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
					Spec: v1.SecuresignSpec{
						Rekor: v1.RekorSpec{
							PodRequirements: v1.PodRequirements{Replicas: ptr.To[int32](2)},
							TreeID:          ptr.To[int64](12345),
							Signer:          v1.RekorSigner{KMS: "secret"},
						},
						Fulcio: v1.FulcioSpec{
							Config: v1.FulcioConfig{
								OIDCIssuers: []v1.OIDCIssuer{
									{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
								},
							},
							Certificate: v1.FulcioCert{OrganizationName: "Red Hat"},
						},
						Trillian: v1.TrillianSpec{
							Db: v1.TrillianDB{Create: ptr.To(true)},
						},
						Ctlog: v1.CTlogSpec{
							TreeID: ptr.To[int64](67890),
						},
						TimestampAuthority: &v1.TimestampAuthoritySpec{
							Signer: v1.TimestampAuthoritySigner{
								CertificateChain: v1.CertificateChain{
									RootCA: &v1.TsaCertificateAuthority{
										OrganizationName: "Red Hat",
									},
								},
							},
						},
					},
				}
			},
			spoke: func() *Securesign {
				return &Securesign{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
					Spec: SecuresignSpec{
						Rekor: RekorSpec{
							PodRequirements: PodRequirements{Replicas: ptr.To[int32](2)},
							TreeID:          ptr.To[int64](12345),
							Signer:          RekorSigner{KMS: "secret"},
						},
						Fulcio: FulcioSpec{
							Config: FulcioConfig{
								OIDCIssuers: []OIDCIssuer{
									{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
								},
							},
							Certificate: FulcioCert{OrganizationName: "Red Hat"},
						},
						Trillian: TrillianSpec{
							Db: TrillianDB{Create: ptr.To(true)},
						},
						Ctlog: CTlogSpec{
							TreeID: ptr.To[int64](67890),
						},
						TimestampAuthority: &TimestampAuthoritySpec{
							Signer: TimestampAuthoritySigner{
								CertificateChain: CertificateChain{
									RootCA: &TsaCertificateAuthority{
										OrganizationName: "Red Hat",
									},
								},
							},
						},
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/v1_to_v1alpha1", func(t *testing.T) {
			hub := tt.hub()
			expectedSpoke := tt.spoke()

			gotSpoke := &Securesign{}
			if err := gotSpoke.ConvertFrom(hub); err != nil {
				t.Fatalf("ConvertFrom failed: %v", err)
			}

			if !equality.Semantic.DeepEqual(expectedSpoke, gotSpoke) {
				t.Errorf("v1 → v1alpha1 mismatch (-want +got):\n%s", cmp.Diff(expectedSpoke, gotSpoke))
			}
		})

		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			spoke := tt.spoke()
			expectedHub := tt.hub()

			gotHub := &v1.Securesign{}
			if err := spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}

			if !equality.Semantic.DeepEqual(expectedHub, gotHub) {
				t.Errorf("v1alpha1 → v1 mismatch (-want +got):\n%s", cmp.Diff(expectedHub, gotHub))
			}
		})
	}
}

func TestCTlogConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *v1.CTlog
		spoke *CTlog
	}{
		{
			name: "basic fields",
			hub: &v1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
				Spec: v1.CTlogSpec{
					TreeID:           ptr.To[int64](999),
					MaxCertChainSize: ptr.To[int64](153600),
					Trillian:         v1.TrillianService{Address: "trillian:8091", Port: ptr.To[int32](8091)},
				},
			},
			spoke: &CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
				Spec: CTlogSpec{
					TreeID:           ptr.To[int64](999),
					MaxCertChainSize: ptr.To[int64](153600),
					Trillian:         TrillianService{Address: "trillian:8091", Port: ptr.To[int32](8091)},
				},
			},
		},
		{
			name: "key refs and root certificates",
			hub: &v1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
				Spec: v1.CTlogSpec{
					PrivateKeyRef:         &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "ctlog-secret"}, Key: "private"},
					PrivateKeyPasswordRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "ctlog-secret"}, Key: "password"},
					PublicKeyRef:          &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "ctlog-secret"}, Key: "public"},
					RootCertificates: []v1.SecretKeySelector{
						{LocalObjectReference: v1.LocalObjectReference{Name: "root-cert"}, Key: "ca.crt"},
					},
				},
			},
			spoke: &CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
				Spec: CTlogSpec{
					PrivateKeyRef:         &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "ctlog-secret"}, Key: "private"},
					PrivateKeyPasswordRef: &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "ctlog-secret"}, Key: "password"},
					PublicKeyRef:          &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "ctlog-secret"}, Key: "public"},
					RootCertificates: []SecretKeySelector{
						{LocalObjectReference: LocalObjectReference{Name: "root-cert"}, Key: "ca.crt"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/v1_to_v1alpha1", func(t *testing.T) {
			gotSpoke := &CTlog{}
			if err := gotSpoke.ConvertFrom(tt.hub); err != nil {
				t.Fatalf("ConvertFrom failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &v1.CTlog{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}

func TestRekorConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *v1.Rekor
		spoke *Rekor
	}{
		{
			name: "basic fields with PVC and attestations",
			hub: &v1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: v1.RekorSpec{
					TreeID:   ptr.To[int64](111),
					Trillian: v1.TrillianService{Address: "trillian:8091", Port: ptr.To[int32](8091)},
					Pvc: v1.Pvc{
						Size:   ptr.To(resource.MustParse("5Gi")),
						Retain: ptr.To(true),
					},
					Attestations: v1.RekorAttestations{
						Enabled: ptr.To(true),
						Url:     "file:///var/run/attestations?no_tmp_dir=true",
					},
					Signer:    v1.RekorSigner{KMS: "secret"},
					TrustedCA: &v1.LocalObjectReference{Name: "trusted-ca"},
				},
			},
			spoke: &Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: RekorSpec{
					TreeID:   ptr.To[int64](111),
					Trillian: TrillianService{Address: "trillian:8091", Port: ptr.To[int32](8091)},
					Pvc: Pvc{
						Size:   ptr.To(resource.MustParse("5Gi")),
						Retain: ptr.To(true),
					},
					Attestations: RekorAttestations{
						Enabled: ptr.To(true),
						Url:     "file:///var/run/attestations?no_tmp_dir=true",
					},
					Signer:    RekorSigner{KMS: "secret"},
					TrustedCA: &LocalObjectReference{Name: "trusted-ca"},
				},
			},
		},
		{
			name: "sharding and search index",
			hub: &v1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: v1.RekorSpec{
					Sharding: []v1.RekorLogRange{
						{TreeID: 100, TreeLength: 50000, EncodedPublicKey: "dGVzdA=="},
					},
					SearchIndex: v1.SearchIndex{Create: ptr.To(true)},
				},
			},
			spoke: &Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: RekorSpec{
					Sharding: []RekorLogRange{
						{TreeID: 100, TreeLength: 50000, EncodedPublicKey: "dGVzdA=="},
					},
					SearchIndex: SearchIndex{Create: ptr.To(true)},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/v1_to_v1alpha1", func(t *testing.T) {
			gotSpoke := &Rekor{}
			if err := gotSpoke.ConvertFrom(tt.hub); err != nil {
				t.Fatalf("ConvertFrom failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &v1.Rekor{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}

func TestFulcioConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *v1.Fulcio
		spoke *Fulcio
	}{
		{
			name: "OIDC issuers and certificate",
			hub: &v1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: "default"},
				Spec: v1.FulcioSpec{
					Config: v1.FulcioConfig{
						OIDCIssuers: []v1.OIDCIssuer{
							{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
							{Issuer: "https://token.actions.githubusercontent.com", ClientID: "sigstore", Type: "github-workflow", CIProvider: "github"},
						},
						MetaIssuers: []v1.OIDCIssuer{
							{Issuer: "https://oidc.eks.*.amazonaws.com/id/*", ClientID: "sigstore", Type: "kubernetes"},
						},
					},
					Certificate: v1.FulcioCert{
						OrganizationName: "Red Hat",
						CommonName:       "fulcio.example.com",
					},
					TrustedCA: &v1.LocalObjectReference{Name: "ca-bundle"},
				},
			},
			spoke: &Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: "default"},
				Spec: FulcioSpec{
					Config: FulcioConfig{
						OIDCIssuers: []OIDCIssuer{
							{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
							{Issuer: "https://token.actions.githubusercontent.com", ClientID: "sigstore", Type: "github-workflow", CIProvider: "github"},
						},
						MetaIssuers: []OIDCIssuer{
							{Issuer: "https://oidc.eks.*.amazonaws.com/id/*", ClientID: "sigstore", Type: "kubernetes"},
						},
					},
					Certificate: FulcioCert{
						OrganizationName: "Red Hat",
						CommonName:       "fulcio.example.com",
					},
					TrustedCA: &LocalObjectReference{Name: "ca-bundle"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/v1_to_v1alpha1", func(t *testing.T) {
			gotSpoke := &Fulcio{}
			if err := gotSpoke.ConvertFrom(tt.hub); err != nil {
				t.Fatalf("ConvertFrom failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &v1.Fulcio{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}

func TestTrillianConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *v1.Trillian
		spoke *Trillian
	}{
		{
			name: "database with TLS and auth",
			hub: &v1.Trillian{
				ObjectMeta: metav1.ObjectMeta{Name: "trillian", Namespace: "default"},
				Spec: v1.TrillianSpec{
					Db: v1.TrillianDB{
						Create:   ptr.To(true),
						Provider: "mysql",
						Pvc: v1.Pvc{
							Size:   ptr.To(resource.MustParse("5Gi")),
							Retain: ptr.To(true),
						},
						TLS: v1.TLS{
							PrivateKeyRef: &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "db-tls"}, Key: "tls.key"},
							CertRef:       &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: "db-tls"}, Key: "tls.crt"},
						},
					},
					MaxRecvMessageSize: ptr.To[int64](153600),
				},
			},
			spoke: &Trillian{
				ObjectMeta: metav1.ObjectMeta{Name: "trillian", Namespace: "default"},
				Spec: TrillianSpec{
					Db: TrillianDB{
						Create:   ptr.To(true),
						Provider: "mysql",
						Pvc: Pvc{
							Size:   ptr.To(resource.MustParse("5Gi")),
							Retain: ptr.To(true),
						},
						TLS: TLS{
							PrivateKeyRef: &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "db-tls"}, Key: "tls.key"},
							CertRef:       &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "db-tls"}, Key: "tls.crt"},
						},
					},
					MaxRecvMessageSize: ptr.To[int64](153600),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/v1_to_v1alpha1", func(t *testing.T) {
			gotSpoke := &Trillian{}
			if err := gotSpoke.ConvertFrom(tt.hub); err != nil {
				t.Fatalf("ConvertFrom failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &v1.Trillian{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}

func TestTufConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *v1.Tuf
		spoke *Tuf
	}{
		{
			name: "keys and service references",
			hub: &v1.Tuf{
				ObjectMeta: metav1.ObjectMeta{Name: "tuf", Namespace: "default"},
				Spec: v1.TufSpec{
					Port: 80,
					Keys: []v1.TufKey{
						{Name: "rekor.pub"},
						{Name: "ctfe.pub"},
						{Name: "fulcio_v1.crt.pem"},
					},
					Ctlog:  v1.CtlogService{Address: "ctlog:6963", Prefix: "trusted-artifact-signer"},
					Fulcio: v1.FulcioService{Address: "fulcio:5554"},
					Rekor:  v1.RekorService{Address: "rekor:3000"},
				},
			},
			spoke: &Tuf{
				ObjectMeta: metav1.ObjectMeta{Name: "tuf", Namespace: "default"},
				Spec: TufSpec{
					Port: 80,
					Keys: []TufKey{
						{Name: "rekor.pub"},
						{Name: "ctfe.pub"},
						{Name: "fulcio_v1.crt.pem"},
					},
					Ctlog:  CtlogService{Address: "ctlog:6963", Prefix: "trusted-artifact-signer"},
					Fulcio: FulcioService{Address: "fulcio:5554"},
					Rekor:  RekorService{Address: "rekor:3000"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/v1_to_v1alpha1", func(t *testing.T) {
			gotSpoke := &Tuf{}
			if err := gotSpoke.ConvertFrom(tt.hub); err != nil {
				t.Fatalf("ConvertFrom failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &v1.Tuf{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}

func TestTimestampAuthorityConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *v1.TimestampAuthority
		spoke *TimestampAuthority
	}{
		{
			name: "KMS signer with auth",
			hub: &v1.TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "default"},
				Spec: v1.TimestampAuthoritySpec{
					Signer: v1.TimestampAuthoritySigner{
						CertificateChain: v1.CertificateChain{
							CertificateChainRef: &v1.SecretKeySelector{
								LocalObjectReference: v1.LocalObjectReference{Name: "tsa-chain"},
								Key:                  "chain.pem",
							},
						},
						Kms: &v1.KMS{
							KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1",
							Auth: &v1.Auth{
								SecretMount: []v1.SecretKeySelector{
									{LocalObjectReference: v1.LocalObjectReference{Name: "gcp-creds"}, Key: "credentials.json"},
								},
							},
						},
					},
					NTPMonitoring: v1.NTPMonitoring{
						Enabled: true,
					},
				},
			},
			spoke: &TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "default"},
				Spec: TimestampAuthoritySpec{
					Signer: TimestampAuthoritySigner{
						CertificateChain: CertificateChain{
							CertificateChainRef: &SecretKeySelector{
								LocalObjectReference: LocalObjectReference{Name: "tsa-chain"},
								Key:                  "chain.pem",
							},
						},
						Kms: &KMS{
							KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1",
							Auth: &Auth{
								SecretMount: []SecretKeySelector{
									{LocalObjectReference: LocalObjectReference{Name: "gcp-creds"}, Key: "credentials.json"},
								},
							},
						},
					},
					NTPMonitoring: NTPMonitoring{
						Enabled: true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/v1_to_v1alpha1", func(t *testing.T) {
			gotSpoke := &TimestampAuthority{}
			if err := gotSpoke.ConvertFrom(tt.hub); err != nil {
				t.Fatalf("ConvertFrom failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &v1.TimestampAuthority{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}
