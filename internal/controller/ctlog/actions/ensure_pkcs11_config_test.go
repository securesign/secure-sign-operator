package actions

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// pkcs11CTlogInstance returns a CTlog instance in Creating state with PKCS#11
// signer type pre-configured. Tests can override fields as needed.
func pkcs11CTlogInstance() *rhtasv1.CTlog {
	return &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ctlog",
			Namespace: "default",
		},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					Prefix: "test-log",
					Active: ptr.To(true),
					Signer: &rhtasv1.CTlogSigner{
						Type: rhtasv1.SignerTypePKCS11,
						PKCS11: &rhtasv1.CTlogPKCS11Config{
							PinSecretRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-pin"},
								Key:                  "pin",
							},
							TokenLabel: "ctlog-token",
							ModulePath: "/usr/lib64/pkcs11/libsofthsm2.so",
							PublicKeyRef: &rhtasv1.SecretKeySelector{
								LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-pubkey"},
								Key:                  "public",
							},
						},
					},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
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

// --- CanHandle tests ---

func TestCanHandle_FileMode(t *testing.T) {
	g := NewWithT(t)
	instance := pkcs11CTlogInstance()
	instance.Spec.Logs[0].Signer.Type = rhtasv1.SignerTypeFile
	instance.Spec.Logs[0].Signer.PKCS11 = nil

	a := NewEnsurePKCS11ConfigAction()
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestCanHandle_PKCS11_NoCond(t *testing.T) {
	g := NewWithT(t)
	instance := pkcs11CTlogInstance()
	// No PKCS11Condition set yet -- CanHandle should return true.

	a := NewEnsurePKCS11ConfigAction()
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestCanHandle_PKCS11_CondTrue_SameHash(t *testing.T) {
	g := NewWithT(t)
	instance := pkcs11CTlogInstance()

	// Pre-compute hash and set annotation + condition.
	hash := pkcs11SpecHash(instance.Spec.Logs[0].Signer.PKCS11)
	instance.SetAnnotations(map[string]string{annotations.PKCS11SpecHash: hash})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    PKCS11Condition,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonResolved,
		Message: PKCS11MessageResolved,
	})

	a := NewEnsurePKCS11ConfigAction()
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestCanHandle_PKCS11_CondTrue_DiffHash(t *testing.T) {
	g := NewWithT(t)
	instance := pkcs11CTlogInstance()

	// Set PKCS11Condition with a stale hash (simulates spec rotation).
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    PKCS11Condition,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonResolved,
		Message: PKCS11MessageResolved,
	})

	a := NewEnsurePKCS11ConfigAction()
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestCanHandle_PKCS11_PendingState(t *testing.T) {
	g := NewWithT(t)
	instance := pkcs11CTlogInstance()
	// Set state to Pending (< Creating) -- CanHandle should return false.
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   constants.ReadyCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Pending.String(),
	})

	a := NewEnsurePKCS11ConfigAction()
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

// --- Handle tests ---

func TestHandle_ValidRefs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := pkcs11CTlogInstance()

	pinSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pin", Namespace: "default"},
		Data:       map[string][]byte{"pin": []byte("1234")},
	}
	pubKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pubkey", Namespace: "default"},
		Data:       map[string][]byte{"public": []byte("-----BEGIN PUBLIC KEY-----\nMFkw...")},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, pinSecret, pubKeySecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	// Should succeed without error.
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	// PKCS11Condition should be True.
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Message).To(Equal(PKCS11MessageResolved))
	g.Expect(instance.GetAnnotations()[annotations.PKCS11SpecHash]).ToNot(BeEmpty())

	// ConfigCondition should be invalidated (set to False) to trigger server config regeneration.
	configCond := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
	g.Expect(configCond).ToNot(BeNil())
	g.Expect(configCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(configCond.Reason).To(Equal("PKCS11Config"))
}

func TestHandle_NilPKCS11(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := pkcs11CTlogInstance()
	instance.Spec.Logs[0].Signer.PKCS11 = nil

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

func TestHandle_MissingPin(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := pkcs11CTlogInstance()

	// Only create the public key secret, not the pin secret.
	pubKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pubkey", Namespace: "default"},
		Data:       map[string][]byte{"public": []byte("-----BEGIN PUBLIC KEY-----\nMFkw...")},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, pubKeySecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	// ReturnOnChange(PersistStatus) returns Return() on status change, not an error result.
	// The PKCS11Condition should be set to False.
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal("SecretNotFound"))
	g.Expect(cond.Message).To(ContainSubstring("pinSecretRef"))
}

func TestHandle_MissingPubKey(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := pkcs11CTlogInstance()

	// Only create the pin secret, not the public key secret.
	pinSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pin", Namespace: "default"},
		Data:       map[string][]byte{"pin": []byte("1234")},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, pinSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	// PKCS11Condition should be set to False.
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal("SecretNotFound"))
	g.Expect(cond.Message).To(ContainSubstring("publicKeyRef"))
}

func TestHandle_SetsPublicKeyRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := pkcs11CTlogInstance()

	pinSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pin", Namespace: "default"},
		Data:       map[string][]byte{"pin": []byte("1234")},
	}
	pubKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pubkey", Namespace: "default"},
		Data:       map[string][]byte{"public": []byte("-----BEGIN PUBLIC KEY-----\nMFkw...")},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, pinSecret, pubKeySecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	// Status.PublicKeyRef should point to the spec's PublicKeyRef.
	g.Expect(instance.Status.PublicKeyRef).ToNot(BeNil())
	g.Expect(instance.Status.PublicKeyRef.Name).To(Equal("hsm-pubkey"))
	g.Expect(instance.Status.PublicKeyRef.Key).To(Equal("public"))
}

func TestHandle_ConfigConditionInvalidated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := pkcs11CTlogInstance()

	// Pre-set ConfigCondition to True to verify it gets invalidated.
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   ConfigCondition,
		Status: metav1.ConditionTrue,
		Reason: ReasonResolved,
	})

	pinSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pin", Namespace: "default"},
		Data:       map[string][]byte{"pin": []byte("1234")},
	}
	pubKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "hsm-pubkey", Namespace: "default"},
		Data:       map[string][]byte{"public": []byte("-----BEGIN PUBLIC KEY-----\nMFkw...")},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, pinSecret, pubKeySecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Err).ToNot(HaveOccurred())

	// ConfigCondition should be False after successful PKCS#11 validation.
	configCond := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
	g.Expect(configCond).ToNot(BeNil())
	g.Expect(configCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(configCond.Reason).To(Equal("PKCS11Config"))
}

func TestHandle_Rotation_FieldsUnchanged(t *testing.T) {
	g := NewWithT(t)
	instance := pkcs11CTlogInstance()

	// Set annotation + condition with matching hash -- CanHandle should return false.
	hash := pkcs11SpecHash(instance.Spec.Logs[0].Signer.PKCS11)
	instance.SetAnnotations(map[string]string{annotations.PKCS11SpecHash: hash})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               PKCS11Condition,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonResolved,
		Message:            PKCS11MessageResolved,
		ObservedGeneration: 1,
	})

	// Bump generation (e.g. monitoring toggle) without changing PKCS#11 fields.
	instance.Generation = 2

	a := NewEnsurePKCS11ConfigAction()
	// CanHandle should be false because PKCS#11 fields have not changed
	// (regression test for osmman issue #1).
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestHandle_Rotation_PinSecretRefChanged(t *testing.T) {
	g := NewWithT(t)
	instance := pkcs11CTlogInstance()

	// Set annotation + condition with hash from old spec.
	oldHash := pkcs11SpecHash(instance.Spec.Logs[0].Signer.PKCS11)
	instance.SetAnnotations(map[string]string{annotations.PKCS11SpecHash: oldHash})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    PKCS11Condition,
		Status:  metav1.ConditionTrue,
		Reason:  ReasonResolved,
		Message: PKCS11MessageResolved,
	})

	// Change PinSecretRef (simulates secret rotation).
	instance.Spec.Logs[0].Signer.PKCS11.PinSecretRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "hsm-pin-rotated"},
		Key:                  "pin",
	}

	a := NewEnsurePKCS11ConfigAction()
	// CanHandle should be true because PinSecretRef changed.
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestHandle_NilPinSecretRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := pkcs11CTlogInstance()
	instance.Spec.Logs[0].Signer.PKCS11.PinSecretRef = nil

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal("Missing"))
	g.Expect(cond.Message).To(ContainSubstring("pinSecretRef"))
}

func TestHandle_EmptyPinSecretData(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11CTlogInstance()
	c := testAction.FakeClientBuilder().
		WithObjects(
			instance,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "hsm-pin", Namespace: "default"},
				Data:       map[string][]byte{"pin": {}},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "ctlog-pubkey", Namespace: "default"},
				Data:       map[string][]byte{"public.pem": []byte("pubkey-data")},
			},
		).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal("EmptySecret"))
}

func TestHandle_MissingPublicKeyRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	instance := pkcs11CTlogInstance()
	c := testAction.FakeClientBuilder().
		WithObjects(
			instance,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "hsm-pin", Namespace: "default"},
				Data:       map[string][]byte{"pin": []byte("testpin")},
			},
		).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewEnsurePKCS11ConfigAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).ToNot(BeNil())
	cond := meta.FindStatusCondition(instance.Status.Conditions, PKCS11Condition)
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal("SecretNotFound"))
}
