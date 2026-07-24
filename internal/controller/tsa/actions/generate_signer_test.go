package actions

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	generateSigner "github.com/securesign/operator/internal/action/generateSigner"
	"github.com/securesign/operator/internal/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/fips"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func tsaInstance() *rhtasv1.TimestampAuthority {
	return &rhtasv1.TimestampAuthority{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tsa",
			Namespace: "default",
		},
		Status: rhtasv1.TimestampAuthorityStatus{
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				{Type: TSASignerCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
			},
		},
		Spec: rhtasv1.TimestampAuthoritySpec{
			Signer: rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{
						{
							OrganizationName: "Red Hat",
						},
					},
					LeafCA: &rhtasv1.TsaCertificateAuthority{
						OrganizationName: "Red Hat",
					},
				},
			},
		},
	}
}

func TestTSASigner_KMSWithCertChainRef(t *testing.T) {
	g := NewWithT(t)
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "kms-secret"},
				Key:                  "certificateChain",
			},
		},
		Kms: &rhtasv1.KMS{
			KeyResource: "projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key/cryptoKeyVersions/1",
		},
	}

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestTSASigner_TinkWithCertChainRef(t *testing.T) {
	g := NewWithT(t)
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tink-secret"},
				Key:                  "certificateChain",
			},
		},
		Tink: &rhtasv1.Tink{
			KeysetRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tink-secret"},
				Key:                  "keySet",
			},
		},
	}

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestTSASigner_TinkWithoutKeysetRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tink-secret"},
				Key:                  "certificateChain",
			},
		},
		Tink: &rhtasv1.Tink{},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())

	result := a.Handle(ctx, instance)
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
	g.Expect(result.Err.Error()).To(ContainSubstring("missing keyset reference"))

	cond := meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition)
	g.Expect(cond).NotTo(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
}

func TestTSASigner_FileEnabled(t *testing.T) {
	g := NewWithT(t)
	instance := tsaInstance()

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestTSASigner_UserProvidedRefs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-cert-secret"},
				Key:                  "certificateChain",
			},
		},
		File: &rhtasv1.File{
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-key-secret"},
				Key:                  "leafPrivateKey",
			},
		},
	}

	keySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-key-secret", Namespace: "default"},
		Data:       map[string][]byte{"leafPrivateKey": []byte("key-data")},
	}
	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-cert-secret", Namespace: "default"},
		Data:       map[string][]byte{"certificateChain": []byte("cert-data")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, keySecret, certSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Signer).NotTo(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("user-cert-secret"))
	g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal("user-key-secret"))
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())
}

func TestTSASigner_UserProvidedRefs_MissingCertChainSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "missing-cert-secret"},
				Key:                  "certificateChain",
			},
		},
		File: &rhtasv1.File{
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-key-secret"},
				Key:                  "leafPrivateKey",
			},
		},
	}

	keySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-key-secret", Namespace: "default"},
		Data:       map[string][]byte{"leafPrivateKey": []byte("key-data")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, keySecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, generateSigner.ErrSecretNotFound)).To(BeTrue())
}

func TestTSASigner_GeneratesData(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Signer).NotTo(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef).NotTo(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("tsa-signer-config-tsa"))
	g.Expect(instance.Status.Signer.FileSigner).NotTo(BeNil(), "FileSigner should be defaulted on status for cert-chain-only spec")
	g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef).NotTo(BeNil())
	g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal("tsa-signer-config-tsa"))

	secret := &corev1.Secret{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tsa-signer-config-tsa", Namespace: "default"},
	}), secret)).To(Succeed())

	g.Expect(secret.Data).To(HaveKey("certificateChain"))
	g.Expect(secret.Data).To(HaveKey("leafPrivateKey"))
	g.Expect(secret.Labels).ToNot(BeEmpty())
	g.Expect(secret.Labels).To(HaveKeyWithValue(labels.LabelNamespace+"/tsa.certchain.pem", tsaUtils.KeyCertificateChain))
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())
}

func TestTSASigner_MigrationFromPreExistingSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	// Upgrade from <1.5.0: status references an old GenerateName-based secret
	instance.Status.Signer = &rhtasv1.TimestampAuthoritySignerStatus{
		CertificateChainRef: &rhtasv1.SecretKeySelector{
			Key:                  "certificateChain",
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tsa-signer-tsa-abc12"},
		},
		FileSigner: &rhtasv1.FileSignerStatus{
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				Key:                  "leafPrivateKey",
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tsa-signer-tsa-abc12"},
			},
		},
	}

	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tsa-signer-tsa-abc12", Namespace: "default"},
		Data: map[string][]byte{
			"certificateChain": []byte("old-chain"),
			"leafPrivateKey":   []byte("old-key"),
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, oldSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	// Status should still reference the OLD secret
	g.Expect(instance.Status.Signer).ToNot(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("tsa-signer-tsa-abc12"))

	// No new deterministic-named secret should have been created
	newSecret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(signerSecretNameFormat, "tsa"), Namespace: "default"},
	}), newSecret)
	g.Expect(err).To(HaveOccurred())
}

func TestTSASigner_DeterministicName(t *testing.T) {
	g := NewWithT(t)
	g.Expect(fmt.Sprintf(signerSecretNameFormat, "my-tsa")).To(Equal("tsa-signer-config-my-tsa"))
}

func TestTSASigner_PasswordRefRejectedInFIPS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-cert-secret"},
				Key:                  "certificateChain",
			},
		},
		File: &rhtasv1.File{
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-key-secret"},
				Key:                  "leafPrivateKey",
			},
			PasswordRef: &rhtasv1.SecretKeySelector{ //nolint:staticcheck
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-password"},
				Key:                  "password",
			},
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewFIPSValidationAction())
	result := a.Handle(ctx, instance)

	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
	g.Expect(result.Err.Error()).To(ContainSubstring("FIPS"))
	cond := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
}

func TestTSASigner_UnencryptedKeyAllowedInFIPS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	unencryptedKey, err := generatePrivateKey()
	g.Expect(err).ToNot(HaveOccurred())

	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-cert-secret"},
				Key:                  "certificateChain",
			},
		},
		File: &rhtasv1.File{
			PrivateKeyRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-key-secret"},
				Key:                  "leafPrivateKey",
			},
		},
	}

	certChainPEM := generateTestCertChainPEM(t)

	keySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-key-secret", Namespace: "default"},
		Data:       map[string][]byte{"leafPrivateKey": unencryptedKey},
	}
	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-cert-secret", Namespace: "default"},
		Data:       map[string][]byte{"certificateChain": certChainPEM},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, keySecret, certSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewFIPSValidationAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
}

func TestTSASigner_AlignStatusFields(t *testing.T) {
	const userSecret = "user-signer-secret"

	tests := []struct {
		name           string
		signer         rhtasv1.TimestampAuthoritySigner
		existingStatus *rhtasv1.TimestampAuthoritySignerStatus
		testCase       func(Gomega, *rhtasv1.TimestampAuthority)
	}{
		{
			name: "default signer (no File, no ChainRef) — defaults to File-based",
			signer: rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA:         &rhtasv1.TsaCertificateAuthority{OrganizationName: "Red Hat"},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{{OrganizationName: "Red Hat"}},
					LeafCA:         &rhtasv1.TsaCertificateAuthority{OrganizationName: "Red Hat"},
				},
			},
			testCase: func(g Gomega, instance *rhtasv1.TimestampAuthority) {
				g.Expect(instance.Status.Signer).NotTo(BeNil())
				g.Expect(instance.Status.Signer.FileSigner).NotTo(BeNil(), "File should be defaulted on Status")
				g.Expect(instance.Status.Signer.CertificateChainRef).NotTo(BeNil())
				g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("test-secret"))
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef).NotTo(BeNil(), "PrivateKeyRef should be defaulted")
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Key).To(Equal("leafPrivateKey"))
				g.Expect(instance.Status.Signer.FileSigner.PasswordRef).To(BeNil()) //nolint:staticcheck
			},
		},
		{
			name: "File signer with user-provided keys — preserves refs",
			signer: rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA:         &rhtasv1.TsaCertificateAuthority{OrganizationName: "Red Hat"},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{{OrganizationName: "Red Hat"}},
					LeafCA:         &rhtasv1.TsaCertificateAuthority{OrganizationName: "Red Hat"},
				},
				File: &rhtasv1.File{
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						Key:                  "myKey",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: userSecret},
					},
					PasswordRef: &rhtasv1.SecretKeySelector{
						Key:                  "myPassword",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: userSecret},
					},
				},
			},
			testCase: func(g Gomega, instance *rhtasv1.TimestampAuthority) {
				g.Expect(instance.Status.Signer.FileSigner).NotTo(BeNil())
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal(userSecret), "should preserve user-provided PrivateKeyRef")
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Key).To(Equal("myKey"))
				g.Expect(instance.Status.Signer.FileSigner.PasswordRef.Name).To(Equal(userSecret), "should preserve user-provided PasswordRef")
				g.Expect(instance.Status.Signer.FileSigner.PasswordRef.Key).To(Equal("myPassword"))
				g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("test-secret"))
			},
		},
		{
			name: "File signer with partial refs — defaults missing",
			signer: rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					RootCA:         &rhtasv1.TsaCertificateAuthority{OrganizationName: "Red Hat"},
					IntermediateCA: []*rhtasv1.TsaCertificateAuthority{{OrganizationName: "Red Hat"}},
					LeafCA:         &rhtasv1.TsaCertificateAuthority{OrganizationName: "Red Hat"},
				},
				File: &rhtasv1.File{
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						Key:                  "myKey",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: userSecret},
					},
					// PasswordRef intentionally nil
				},
			},
			testCase: func(g Gomega, instance *rhtasv1.TimestampAuthority) {
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal(userSecret), "should keep user-provided PrivateKeyRef")
				g.Expect(instance.Status.Signer.FileSigner.PasswordRef).To(BeNil()) //nolint:staticcheck
			},
		},
		{
			name: "preserves existing status refs when non-nil",
			signer: rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					CertificateChainRef: &rhtasv1.SecretKeySelector{
						Key:                  "chain",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
					},
				},
				File: &rhtasv1.File{
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						Key:                  "key",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
					},
				},
			},
			existingStatus: &rhtasv1.TimestampAuthoritySignerStatus{
				CertificateChainRef: &rhtasv1.SecretKeySelector{
					Key:                  "chain",
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: "existing-secret"},
				},
				FileSigner: &rhtasv1.FileSignerStatus{
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						Key:                  "key",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "existing-secret"},
					},
				},
			},
			testCase: func(g Gomega, instance *rhtasv1.TimestampAuthority) {
				g.Expect(instance.Status.Signer).NotTo(BeNil())
				g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("user-secret"),
					"should reflect spec CertificateChainRef")
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal("user-secret"),
					"should reflect spec PrivateKeyRef")
			},
		},
		{
			name: "external CertificateChainRef with File signer — skips defaults",
			signer: rhtasv1.TimestampAuthoritySigner{
				CertificateChain: rhtasv1.CertificateChain{
					CertificateChainRef: &rhtasv1.SecretKeySelector{
						Key:                  "chain",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "external-secret"},
					},
				},
				File: &rhtasv1.File{
					PrivateKeyRef: &rhtasv1.SecretKeySelector{
						Key:                  "key",
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "external-secret"},
					},
				},
			},
			testCase: func(g Gomega, instance *rhtasv1.TimestampAuthority) {
				g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("external-secret"), "should preserve external ChainRef")
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal("external-secret"), "should preserve external PrivateKeyRef")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := &rhtasv1.TimestampAuthority{
				Spec: rhtasv1.TimestampAuthoritySpec{
					Signer: tt.signer,
				},
			}
			if tt.existingStatus != nil {
				instance.Status.Signer = tt.existingStatus.DeepCopy()
			}
			alignStatus(instance, rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "test-secret"}})
			tt.testCase(g, instance)
		})
	}
}

func generateTestCertChainPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:       big.NewInt(1),
		Subject:            pkix.Name{CommonName: "test-tsa-ca"},
		NotBefore:          time.Now(),
		NotAfter:           time.Now().Add(time.Hour),
		IsCA:               true,
		KeyUsage:           x509.KeyUsageCertSign,
		SignatureAlgorithm: x509.ECDSAWithSHA384,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
