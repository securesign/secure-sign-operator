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
	rhtasv1 "github.com/securesign/operator/api/v1"
	utilconversion "github.com/securesign/operator/internal/conversion"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// stripConversionData removes the conversion-data annotation that MarshalData
// adds during hub->spoke conversion, so unit tests can compare spoke objects.
func stripConversionData(obj metav1.Object) {
	a := obj.GetAnnotations()
	delete(a, utilconversion.DataAnnotation)
	if len(a) == 0 {
		a = nil
	}
	obj.SetAnnotations(a)
}

// conversionTestCase defines a single conversion test case.
type conversionTestCase struct {
	name  string
	hub   func() *rhtasv1.Securesign
	spoke func() *Securesign
}

func TestSecuresignConversionUnit(t *testing.T) {
	tests := []conversionTestCase{
		{
			name: "empty spec round-trips",
			hub: func() *rhtasv1.Securesign {
				return &rhtasv1.Securesign{
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
			hub: func() *rhtasv1.Securesign {
				return &rhtasv1.Securesign{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ns"},
					Spec: rhtasv1.SecuresignSpec{
						Rekor: rhtasv1.RekorSpec{
							PodRequirements: rhtasv1.PodRequirements{Replicas: ptr.To[int32](2)},
							TreeID:          ptr.To[int64](12345),
							Signer:          rhtasv1.RekorSigner{KMS: "secret"},
						},
						Fulcio: rhtasv1.FulcioSpec{
							Config: rhtasv1.FulcioConfig{
								OIDCIssuers: []rhtasv1.OIDCIssuer{
									{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
								},
							},
							Certificate: rhtasv1.FulcioCert{OrganizationName: "Red Hat"},
						},
						Trillian: rhtasv1.TrillianSpec{
							Db: rhtasv1.TrillianDB{Create: ptr.To(true)},
						},
						Ctlog: rhtasv1.CTlogSpec{
							TreeID: ptr.To[int64](67890),
						},
						TimestampAuthority: &rhtasv1.TimestampAuthoritySpec{
							Signer: rhtasv1.TimestampAuthoritySigner{
								CertificateChain: rhtasv1.CertificateChain{
									RootCA: &rhtasv1.TsaCertificateAuthority{
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

			stripConversionData(gotSpoke)
			if !equality.Semantic.DeepEqual(expectedSpoke, gotSpoke) {
				t.Errorf("v1 -> v1alpha1 mismatch (-want +got):\n%s", cmp.Diff(expectedSpoke, gotSpoke))
			}
		})

		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			spoke := tt.spoke()
			expectedHub := tt.hub()

			gotHub := &rhtasv1.Securesign{}
			if err := spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}

			if !equality.Semantic.DeepEqual(expectedHub, gotHub) {
				t.Errorf("v1alpha1 -> v1 mismatch (-want +got):\n%s", cmp.Diff(expectedHub, gotHub))
			}
		})
	}
}

func TestCTlogConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *rhtasv1.CTlog
		spoke *CTlog
	}{
		{
			name: "basic fields",
			hub: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
				Spec: rhtasv1.CTlogSpec{
					TreeID:           ptr.To[int64](999),
					MaxCertChainSize: ptr.To[int64](153600),
					Trillian:         rhtasv1.TrillianService{Address: "trillian:8091", Port: ptr.To[int32](8091)},
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
			hub: &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog", Namespace: "default"},
				Spec: rhtasv1.CTlogSpec{
					PrivateKeyRef:         &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-secret"}, Key: "private"},
					PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-secret"}, Key: "password"},
					PublicKeyRef:          &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-secret"}, Key: "public"},
					RootCertificates: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "root-cert"}, Key: "ca.crt"},
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
			stripConversionData(gotSpoke)
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &rhtasv1.CTlog{}
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
		hub   *rhtasv1.Rekor
		spoke *Rekor
	}{
		{
			name: "basic fields with PVC and attestations",
			hub: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: rhtasv1.RekorSpec{
					TreeID:   ptr.To[int64](111),
					Trillian: rhtasv1.TrillianService{Address: "trillian:8091", Port: ptr.To[int32](8091)},
					Attestations: rhtasv1.RekorAttestations{
						Enabled: ptr.To(true),
						Url:     "file:///var/run/attestations?no_tmp_dir=true",
						Pvc: rhtasv1.Pvc{
							Size:   ptr.To(resource.MustParse("5Gi")),
							Retain: ptr.To(true),
						},
					},
					Signer:    rhtasv1.RekorSigner{KMS: "secret"},
					TrustedCA: &rhtasv1.LocalObjectReference{Name: "trusted-ca"},
				},
			},
			spoke: &Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: RekorSpec{
					TreeID:   ptr.To[int64](111),
					Trillian: TrillianService{Address: "trillian:8091", Port: ptr.To[int32](8091)},
					Attestations: RekorAttestations{
						Enabled: ptr.To(true),
						Url:     "file:///var/run/attestations?no_tmp_dir=true",
					},
					// For backward compatibility, spec.pvc is populated from v1 spec.attestations.pvc
					// so that v1alpha1 clients using the deprecated field can still access the data
					Pvc: Pvc{
						Size:   ptr.To(resource.MustParse("5Gi")),
						Retain: ptr.To(true),
					},
					Signer:    RekorSigner{KMS: "secret"},
					TrustedCA: &LocalObjectReference{Name: "trusted-ca"},
				},
			},
		},
		{
			name: "status signer cross-type conversion",
			hub: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Status: rhtasv1.RekorStatus{
					Signer: rhtasv1.RekorSignerStatus{
						KeyRef:      &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-key"}, Key: "private"},
						PasswordRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-key"}, Key: "password"},
					},
				},
			},
			spoke: &Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Status: RekorStatus{
					Signer: RekorSigner{
						KeyRef:      &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "signer-key"}, Key: "private"},
						PasswordRef: &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "signer-key"}, Key: "password"},
					},
				},
			},
		},
		{
			name: "sharding and search index",
			hub: &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
				Spec: rhtasv1.RekorSpec{
					Sharding: []rhtasv1.RekorLogRange{
						{TreeID: 100, TreeLength: 50000, EncodedPublicKey: "dGVzdA=="},
					},
					SearchIndex: rhtasv1.SearchIndex{Create: ptr.To(true)},
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
			stripConversionData(gotSpoke)
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &rhtasv1.Rekor{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}

// TestRekorPvcMigration specifically tests the migration from old v1alpha1 spec.pvc to new v1 spec.attestations.pvc
func TestRekorPvcMigration(t *testing.T) {
	t.Run("Empty attestations.pvc", func(t *testing.T) {
		// Create a v1alpha1 Rekor with OLD spec.pvc (pre-migration format)
		oldSpoke := &Rekor{
			ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
			Spec: RekorSpec{
				TreeID: ptr.To[int64](333),
				// OLD location: spec.pvc (this is what existing v1alpha1 users have)
				Pvc: Pvc{
					Size:   ptr.To(resource.MustParse("20Gi")),
					Retain: ptr.To(true),
					Name:   "legacy-pvc",
				},
				// Empty attestations.pvc (user hasn't migrated yet)
				Attestations: RekorAttestations{
					Enabled: ptr.To(true),
					Url:     "file:///var/run/attestations?no_tmp_dir=true",
					// Note: Pvc is empty/default
				},
			},
		}

		// Convert v1alpha1 → v1
		hub := &rhtasv1.Rekor{}
		if err := oldSpoke.ConvertTo(hub); err != nil {
			t.Fatalf("ConvertTo failed: %v", err)
		}

		// Verify the PVC was migrated to attestations.pvc in v1
		if hub.Spec.Attestations.Pvc.Size == nil {
			t.Error("Expected attestations.pvc.size to be set after migration")
		} else if hub.Spec.Attestations.Pvc.Size.String() != "20Gi" {
			t.Errorf("Expected attestations.pvc.size = 20Gi, got %s", hub.Spec.Attestations.Pvc.Size.String())
		}

		if hub.Spec.Attestations.Pvc.Name != "legacy-pvc" {
			t.Errorf("Expected attestations.pvc.name = legacy-pvc, got %s", hub.Spec.Attestations.Pvc.Name)
		}

		if hub.Spec.Attestations.Pvc.Retain == nil || !*hub.Spec.Attestations.Pvc.Retain {
			t.Error("Expected attestations.pvc.retain = true after migration")
		}
	})

	t.Run("Both spec.pvc and attestations.pvc set (with defaults)", func(t *testing.T) {
		// Simulate what happens when kubebuilder defaults are applied:
		// User sets spec.pvc.size = 25Gi, but API server applies default attestations.pvc.size = 5Gi
		oldSpoke := &Rekor{
			ObjectMeta: metav1.ObjectMeta{Name: "rekor", Namespace: "default"},
			Spec: RekorSpec{
				TreeID: ptr.To[int64](444),
				// User explicitly set spec.pvc to 25Gi
				Pvc: Pvc{
					Size:   ptr.To(resource.MustParse("25Gi")),
					Retain: ptr.To(true),
				},
				// API server applied kubebuilder defaults to attestations (but pvc is at spec level in v1alpha1)
				Attestations: RekorAttestations{
					Enabled: ptr.To(true),
					Url:     "file:///var/run/attestations?no_tmp_dir=true",
				},
			},
		}

		// Convert v1alpha1 → v1
		hub := &rhtasv1.Rekor{}
		if err := oldSpoke.ConvertTo(hub); err != nil {
			t.Fatalf("ConvertTo failed: %v", err)
		}

		// Verify that spec.pvc (25Gi) takes precedence over the default attestations.pvc (5Gi)
		if hub.Spec.Attestations.Pvc.Size == nil {
			t.Error("Expected attestations.pvc.size to be set after migration")
		} else if hub.Spec.Attestations.Pvc.Size.String() != "25Gi" {
			t.Errorf("Expected attestations.pvc.size = 25Gi (from spec.pvc), got %s", hub.Spec.Attestations.Pvc.Size.String())
		}

		if hub.Spec.Attestations.Pvc.Retain == nil || !*hub.Spec.Attestations.Pvc.Retain {
			t.Error("Expected attestations.pvc.retain = true after migration")
		}
	})
}

func TestFulcioConversionUnit(t *testing.T) {
	tests := []struct {
		name  string
		hub   *rhtasv1.Fulcio
		spoke *Fulcio
	}{
		{
			name: "OIDC issuers and certificate",
			hub: &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: "default"},
				Spec: rhtasv1.FulcioSpec{
					Config: rhtasv1.FulcioConfig{
						OIDCIssuers: []rhtasv1.OIDCIssuer{
							{Issuer: "https://accounts.google.com", ClientID: "sigstore", Type: "email"},
							{Issuer: "https://token.actions.githubusercontent.com", ClientID: "sigstore", Type: "github-workflow", CIProvider: "github"},
						},
						MetaIssuers: []rhtasv1.OIDCIssuer{
							{Issuer: "https://oidc.eks.*.amazonaws.com/id/*", ClientID: "sigstore", Type: "kubernetes"},
						},
					},
					Certificate: rhtasv1.FulcioCert{
						OrganizationName: "Red Hat",
						CommonName:       "fulcio.example.com",
					},
					TrustedCA: &rhtasv1.LocalObjectReference{Name: "ca-bundle"},
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
		{
			name: "fully populated status",
			hub: &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: "ns"},
				Status: rhtasv1.FulcioStatus{
					ServerConfigRef: &rhtasv1.LocalObjectReference{Name: "fulcio-config"},
					Url:             "https://fulcio.rhtas.example.com",
					Certificate: &rhtasv1.FulcioCertStatus{
						PrivateKeyRef:         &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-keys"}, Key: "private"},
						PrivateKeyPasswordRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-keys"}, Key: "password"},
						CARef:                 &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-keys"}, Key: "cert"},
						CommonName:            "fulcio.apps.cluster.example.com",
					},
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Deployed", Message: "all good"},
					},
				},
			},
			spoke: &Fulcio{
				ObjectMeta: metav1.ObjectMeta{Name: "fulcio", Namespace: "ns"},
				Status: FulcioStatus{
					ServerConfigRef: &LocalObjectReference{Name: "fulcio-config"},
					Url:             "https://fulcio.rhtas.example.com",
					Certificate: &FulcioCert{
						PrivateKeyRef:         &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "fulcio-keys"}, Key: "private"},
						PrivateKeyPasswordRef: &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "fulcio-keys"}, Key: "password"},
						CARef:                 &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "fulcio-keys"}, Key: "cert"},
						CommonName:            "fulcio.apps.cluster.example.com",
					},
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Deployed", Message: "all good"},
					},
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
			stripConversionData(gotSpoke)
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &rhtasv1.Fulcio{}
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
		hub   *rhtasv1.Trillian
		spoke *Trillian
	}{
		{
			name: "database with TLS and auth",
			hub: &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{Name: "trillian", Namespace: "default"},
				Spec: rhtasv1.TrillianSpec{
					Db: rhtasv1.TrillianDB{
						Create:   ptr.To(true),
						Provider: "mysql",
						Pvc: rhtasv1.Pvc{
							Size:   ptr.To(resource.MustParse("5Gi")),
							Retain: ptr.To(true),
						},
						TLS: rhtasv1.TLS{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "db-tls"}, Key: "tls.key"},
							CertRef:       &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "db-tls"}, Key: "tls.crt"},
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
		{
			name: "fully populated status",
			hub: &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{Name: "trillian", Namespace: "default"},
				Status: rhtasv1.TrillianStatus{
					Db: rhtasv1.TrillianDBStatus{
						PvcName:           "trillian-db",
						DatabaseSecretRef: &rhtasv1.LocalObjectReference{Name: "db-credentials"},
						TLS: rhtasv1.TLS{
							CertRef:       &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "db-tls"}, Key: "tls.crt"},
							PrivateKeyRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "db-tls"}, Key: "tls.key"},
						},
					},
					LogServer: rhtasv1.TrillianServiceStatus{
						TLS: rhtasv1.TLS{
							CertRef:       &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "server-tls"}, Key: "tls.crt"},
							PrivateKeyRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "server-tls"}, Key: "tls.key"},
						},
					},
					LogSigner: rhtasv1.TrillianServiceStatus{
						TLS: rhtasv1.TLS{
							CertRef:       &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-tls"}, Key: "tls.crt"},
							PrivateKeyRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "signer-tls"}, Key: "tls.key"},
						},
					},
				},
			},
			spoke: &Trillian{
				ObjectMeta: metav1.ObjectMeta{Name: "trillian", Namespace: "default"},
				Status: TrillianStatus{
					Db: TrillianDB{
						Pvc:               Pvc{Name: "trillian-db"},
						DatabaseSecretRef: &LocalObjectReference{Name: "db-credentials"},
						TLS: TLS{
							CertRef:       &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "db-tls"}, Key: "tls.crt"},
							PrivateKeyRef: &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "db-tls"}, Key: "tls.key"},
						},
					},
					LogServer: TrillianLogServer{
						TLS: TLS{
							CertRef:       &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "server-tls"}, Key: "tls.crt"},
							PrivateKeyRef: &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "server-tls"}, Key: "tls.key"},
						},
					},
					LogSigner: TrillianLogSigner{
						TLS: TLS{
							CertRef:       &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "signer-tls"}, Key: "tls.crt"},
							PrivateKeyRef: &SecretKeySelector{LocalObjectReference: LocalObjectReference{Name: "signer-tls"}, Key: "tls.key"},
						},
					},
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
			stripConversionData(gotSpoke)
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &rhtasv1.Trillian{}
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
		hub   *rhtasv1.Tuf
		spoke *Tuf
	}{
		{
			name: "keys and service references",
			hub: &rhtasv1.Tuf{
				ObjectMeta: metav1.ObjectMeta{Name: "tuf", Namespace: "default"},
				Spec: rhtasv1.TufSpec{
					Port: 80,
					Keys: []rhtasv1.TufKey{
						{Name: "rekor.pub"},
						{Name: "ctfe.pub"},
						{Name: "fulcio_v1.crt.pem"},
					},
					Ctlog:  rhtasv1.CtlogService{Address: "ctlog:6963", Prefix: "trusted-artifact-signer"},
					Fulcio: rhtasv1.FulcioService{Address: "fulcio:5554"},
					Rekor:  rhtasv1.RekorService{Address: "rekor:3000"},
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
			stripConversionData(gotSpoke)
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &rhtasv1.Tuf{}
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
		hub   *rhtasv1.TimestampAuthority
		spoke *TimestampAuthority
	}{
		{
			name: "fully populated status",
			hub: &rhtasv1.TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "ns"},
				Status: rhtasv1.TimestampAuthorityStatus{
					Url: "https://tsa.rhtas.example.com",
					Signer: &rhtasv1.TimestampAuthoritySignerStatus{
						CertificateChainRef: &rhtasv1.SecretKeySelector{
							LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tsa-chain-secret"},
							Key:                  "chain.pem",
						},
						FileSigner: &rhtasv1.FileSignerStatus{
							PrivateKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tsa-signer-secret"},
								Key:                  "private.key",
							},
							PasswordRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tsa-signer-secret"},
								Key:                  "password",
							},
						},
					},
					NtpConfigRef: &rhtasv1.LocalObjectReference{Name: "ntp-config-map"},
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Deployed", Message: "all good"},
					},
				},
			},
			spoke: &TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "ns"},
				Status: TimestampAuthorityStatus{
					Url: "https://tsa.rhtas.example.com",
					Signer: &TimestampAuthoritySigner{
						CertificateChain: CertificateChain{
							CertificateChainRef: &SecretKeySelector{
								LocalObjectReference: LocalObjectReference{Name: "tsa-chain-secret"},
								Key:                  "chain.pem",
							},
						},
						File: &File{
							PrivateKeyRef: &SecretKeySelector{
								LocalObjectReference: LocalObjectReference{Name: "tsa-signer-secret"},
								Key:                  "private.key",
							},
							PasswordRef: &SecretKeySelector{
								LocalObjectReference: LocalObjectReference{Name: "tsa-signer-secret"},
								Key:                  "password",
							},
						},
					},
					NTPMonitoring: &NTPMonitoring{
						Config: &NtpMonitoringConfig{
							NtpConfigRef: &LocalObjectReference{Name: "ntp-config-map"},
						},
					},
					Conditions: []metav1.Condition{
						{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Deployed", Message: "all good"},
					},
				},
			},
		},
		{
			name: "KMS signer with auth",
			hub: &rhtasv1.TimestampAuthority{
				ObjectMeta: metav1.ObjectMeta{Name: "tsa", Namespace: "default"},
				Spec: rhtasv1.TimestampAuthoritySpec{
					Signer: rhtasv1.TimestampAuthoritySigner{
						CertificateChain: rhtasv1.CertificateChain{
							CertificateChainRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tsa-chain"},
								Key:                  "chain.pem",
							},
						},
						Kms: &rhtasv1.KMS{
							KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1",
							Auth: &rhtasv1.Auth{
								SecretMount: []rhtasv1.SecretKeySelector{
									{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "gcp-creds"}, Key: "credentials.json"},
								},
							},
						},
					},
					NTPMonitoring: rhtasv1.NTPMonitoring{
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
			stripConversionData(gotSpoke)
			if !equality.Semantic.DeepEqual(tt.spoke, gotSpoke) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.spoke, gotSpoke))
			}
		})
		t.Run(tt.name+"/v1alpha1_to_v1", func(t *testing.T) {
			gotHub := &rhtasv1.TimestampAuthority{}
			if err := tt.spoke.ConvertTo(gotHub); err != nil {
				t.Fatalf("ConvertTo failed: %v", err)
			}
			if !equality.Semantic.DeepEqual(tt.hub, gotHub) {
				t.Errorf("mismatch (-want +got):\n%s", cmp.Diff(tt.hub, gotHub))
			}
		})
	}
}
