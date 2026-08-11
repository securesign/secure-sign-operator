package actions

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// pkcs11Instance returns a Fulcio instance in the Creating phase with a valid
// PKCS#11 signer configuration.
func pkcs11Instance() *rhtasv1.Fulcio {
	return &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-fulcio",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: rhtasv1.FulcioSpec{
			Signer: rhtasv1.FulcioSigner{
				Type: rhtasv1.FulcioSignerTypePKCS11,
				PKCS11: &rhtasv1.FulcioPKCS11Config{
					PKCS11Config: rhtasv1.PKCS11Config{
						KeyID:    ptr.To(int32(1)),
						KeyLabel: "signing-key",
					},
					ConfigRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-config"},
						Key:                  "crypto11.conf",
					},
				},
				CertificateChain: rhtasv1.FulcioCertificateChain{
					CertificateChainRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ca-cert"},
						Key:                  "cert.pem",
					},
				},
			},
		},
		Status: rhtasv1.FulcioStatus{
			Conditions: []metav1.Condition{
				{
					Type:   constants.ReadyCondition,
					Status: metav1.ConditionFalse,
					Reason: state.Creating.String(),
				},
			},
		},
	}
}

func TestCanHandle_FileMode(t *testing.T) {
	g := NewWithT(t)

	instance := pkcs11Instance()
	instance.Spec.Signer.Type = rhtasv1.FulcioSignerTypeFile
	instance.Spec.Signer.PKCS11 = nil

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestCanHandle_PKCS11_NoCond(t *testing.T) {
	g := NewWithT(t)

	instance := pkcs11Instance()
	// Instance is in Creating phase with no PKCS11Condition set.

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestCanHandle_PKCS11_CondTrue_SameHash(t *testing.T) {
	g := NewWithT(t)

	instance := pkcs11Instance()
	// Compute the hash that matches the current PKCS#11 config.
	currentHash := computePKCS11Hash(instance)

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		Message:            fmt.Sprintf("PKCS#11 config resolved [hash=%s]", currentHash),
		ObservedGeneration: instance.Generation,
	})

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestCanHandle_PKCS11_CondTrue_DiffHash(t *testing.T) {
	g := NewWithT(t)

	instance := pkcs11Instance()

	// Set PKCS11Condition with an old hash that doesn't match.
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		Message:            "PKCS#11 config resolved [hash=stale-old-hash]",
		ObservedGeneration: instance.Generation,
	})

	// Now change the ConfigRef to trigger hash mismatch.
	instance.Spec.Signer.PKCS11.ConfigRef.Name = "new-hsm-config"

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestHandle_ValidConfig(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11Instance()

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "hsm-config", Namespace: "default"},
				Data:       map[string][]byte{"crypto11.conf": []byte(`{"TokenLabel":"mytoken"}`)},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "ca-cert", Namespace: "default"},
				Data:       map[string][]byte{"cert.pem": []byte("cert-data")},
			},
		).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, PKCS11Condition)).To(BeTrue())

	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	g.Expect(cond.Message).To(ContainSubstring("[hash="))
}

func TestHandle_NilPKCS11(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11Instance()
	instance.Spec.Signer.PKCS11 = nil

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
}

func TestHandle_MissingSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11Instance()

	// No secret registered -- ConfigRef points to a non-existent secret.
	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	// Not a terminal error -- should be retriable.
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeFalse())
}

func TestHandle_SetsCARef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11Instance()

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "hsm-config", Namespace: "default"},
				Data:       map[string][]byte{"crypto11.conf": []byte(`{"TokenLabel":"mytoken"}`)},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "ca-cert", Namespace: "default"},
				Data:       map[string][]byte{"cert.pem": []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n")},
			},
		).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	// Verify Status.Certificate.CARef is set from CertificateChainRef.
	g.Expect(instance.Status.Certificate).ToNot(BeNil())
	g.Expect(instance.Status.Certificate.CARef).ToNot(BeNil())
	g.Expect(instance.Status.Certificate.CARef.Name).To(Equal("ca-cert"))
	g.Expect(instance.Status.Certificate.CARef.Key).To(Equal("cert.pem"))

	// Status.CertificateChain must NOT be pre-populated here.
	// ResolvePubKey fetches it from the running Fulcio API with
	// ParseTrustBundle normalization. Pre-populating with raw secret PEM
	// would cause false drift detection due to whitespace differences.
	g.Expect(instance.Status.CertificateChain).To(BeEmpty())
}

func TestCanHandle_PKCS11_StatePending(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11Instance()
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.ReadyCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Pending.String(),
	})

	a := testAction.PrepareAction(nil, NewEnsurePKCS11ConfigAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeFalse(), "should not handle in Pending state")
}

func TestHandle_NilConfigRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11Instance()
	instance.Spec.Signer.PKCS11.ConfigRef = nil

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(nil))).To(BeTrue())
}

func TestHandle_NilCertificateChainRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11Instance()
	instance.Spec.Signer.CertificateChain.CertificateChainRef = nil

	c := testAction.FakeClientBuilder().
		WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "hsm-config", Namespace: "default"},
			Data:       map[string][]byte{"crypto11.conf": []byte(`{"Path":"/usr/lib/pkcs11/lib.so"}`)},
		}).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(nil))).To(BeTrue())
}
