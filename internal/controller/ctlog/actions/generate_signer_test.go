package actions

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/fips"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ctlogInstance() *rhtasv1.CTlog {
	return &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance",
			Namespace: "default",
		},
		Spec: rhtasv1.CTlogSpec{
			Logs: []rhtasv1.CTLogConfig{
				{
					LogId:  ptr.To(int64(123456)),
					Prefix: "test-log",
					Signer: &rhtasv1.CTlogSigner{Type: "file"},
					Roots: []rhtasv1.SecretKeySelector{
						{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "root"}, Key: "cert"},
					},
				},
			},
		},
		Status: rhtasv1.CTlogStatus{
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				{Type: SignerCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
			},
		},
	}
}

func TestCTlogKeys_AlwaysEnabled(t *testing.T) {
	g := NewWithT(t)
	instance := ctlogInstance()

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestCTlogKeys_UserProvidedKeyRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := ctlogInstance()
	instance.Spec.Logs[0].Signer.File = &rhtasv1.CTlogFile{
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
			Key:                  "private",
		},
		PublicKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
			Key:                  "public",
		},
	}

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-secret", Namespace: "default"},
		Data:       map[string][]byte{"private": []byte("key"), "public": []byte("pub"), "password": []byte("pass")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, userSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.PrivateKeyRef.Name).To(Equal("user-secret"))
	g.Expect(instance.Status.PublicKeyRef.Name).To(Equal("user-secret"))
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition)).To(BeTrue())

	// Config condition should be invalidated
	configCond := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
	g.Expect(configCond).ToNot(BeNil())
	g.Expect(configCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(configCond.Reason).To(Equal(SignerKeyReason))
}

func TestCTlogKeys_UserProvidedPrivateKeyOnly_DerivesPublicKey(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := ctlogInstance()
	instance.Spec.Logs[0].Signer.File = &rhtasv1.CTlogFile{
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
			Key:                  "private",
		},
	}

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-secret", Namespace: "default"},
		Data:       map[string][]byte{"private": []byte("key")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, userSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.PrivateKeyRef.Name).To(Equal("user-secret"))
	g.Expect(instance.Status.PublicKeyRef).ToNot(BeNil())
	g.Expect(instance.Status.PublicKeyRef.Name).To(Equal("user-secret"))
	g.Expect(instance.Status.PublicKeyRef.Key).To(Equal(constants.KeyPublic))
}

func TestCTlogKeys_GeneratesCorrectKeyData(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := ctlogInstance()

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.PrivateKeyRef).ToNot(BeNil())
	g.Expect(instance.Status.PrivateKeyRef.Name).To(Equal("ctlog-keys-config-instance"))
	g.Expect(instance.Status.PrivateKeyRef.Key).To(Equal(constants.KeyPrivate))
	g.Expect(instance.Status.PublicKeyRef).ToNot(BeNil())
	g.Expect(instance.Status.PublicKeyRef.Name).To(Equal("ctlog-keys-config-instance"))
	g.Expect(instance.Status.PublicKeyRef.Key).To(Equal(constants.KeyPublic))

	secret := &corev1.Secret{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ctlog-keys-config-instance", Namespace: "default"},
	}), secret)).To(Succeed())

	g.Expect(secret.Data).To(HaveKey(constants.KeyPrivate))
	g.Expect(secret.Data).To(HaveKey(constants.KeyPublic))
	g.Expect(secret.Data[constants.KeyPrivate]).To(ContainSubstring("EC PRIVATE KEY"))
	g.Expect(secret.Data[constants.KeyPublic]).To(ContainSubstring("PUBLIC KEY"))
	g.Expect(secret.Labels).ToNot(BeEmpty())
	g.Expect(secret.Labels).To(HaveKeyWithValue(labels.LabelNamespace+"/ctfe.pub", constants.KeyPublic))

	// Config condition should be invalidated
	configCond := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
	g.Expect(configCond).ToNot(BeNil())
	g.Expect(configCond.Status).To(Equal(metav1.ConditionFalse))
}

func TestCTlogKeys_MigrationFromPreExistingSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := ctlogInstance()
	// Upgrade from <1.5.0: status references an old GenerateName-based secret
	instance.Status.PrivateKeyRef = &rhtasv1.SecretKeySelector{
		Key:                  constants.KeyPrivate,
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-keys-instance-xyz99"},
	}
	instance.Status.PublicKeyRef = &rhtasv1.SecretKeySelector{
		Key:                  constants.KeyPublic,
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "ctlog-keys-instance-xyz99"},
	}

	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ctlog-keys-instance-xyz99", Namespace: "default"},
		Data: map[string][]byte{
			constants.KeyPrivate: []byte("old-key"),
			constants.KeyPublic:  []byte("old-pub"),
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
	g.Expect(instance.Status.PrivateKeyRef).ToNot(BeNil())
	g.Expect(instance.Status.PrivateKeyRef.Name).To(Equal("ctlog-keys-instance-xyz99"))

	// No new deterministic-named secret should have been created
	newSecret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(signerSecretNameFormat, "instance"), Namespace: "default"},
	}), newSecret)
	g.Expect(err).To(HaveOccurred())
}

func TestCTlogKeys_DeterministicName(t *testing.T) {
	g := NewWithT(t)
	g.Expect(fmt.Sprintf(signerSecretNameFormat, "my-ctlog")).To(Equal("ctlog-keys-config-my-ctlog"))
}

func TestCTlogKeys_AlignStatus_PreservesPrivateKeyPasswordRefWhenPrivateKeyRefUnchanged(t *testing.T) {
	g := NewWithT(t)
	instance := ctlogInstance()

	passwordRef := &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "legacy-secret"},
		Key:                  "password",
	}
	privateKeyRef := &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "private",
	}

	instance.Status.PrivateKeyPasswordRef = passwordRef
	instance.Status.PrivateKeyRef = privateKeyRef

	// Spec points to the SAME key as status
	instance.Spec.Logs[0].Signer.File = &rhtasv1.CTlogFile{
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
			Key:                  "private",
		},
	}

	alignStatus(instance, rhtasv1.SecretKeySelector{})

	g.Expect(instance.Status.PrivateKeyPasswordRef).ToNot(BeNil())
	g.Expect(instance.Status.PrivateKeyPasswordRef.Name).To(Equal("legacy-secret"))
	g.Expect(instance.Status.PrivateKeyPasswordRef.Key).To(Equal("password"))
}

func TestCTlogKeys_AlignStatus_DropsPrivateKeyPasswordRefWhenPrivateKeyRefChanges(t *testing.T) {
	g := NewWithT(t)
	instance := ctlogInstance()

	passwordRef := &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "legacy-secret"},
		Key:                  "password",
	}
	privateKeyRef := &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-secret"},
		Key:                  "private",
	}

	instance.Status.PrivateKeyPasswordRef = passwordRef
	instance.Status.PrivateKeyRef = privateKeyRef

	// Spec points to a DIFFERENT key than status
	instance.Spec.Logs[0].Signer.File = &rhtasv1.CTlogFile{
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "new-secret"},
			Key:                  "private",
		},
	}

	alignStatus(instance, rhtasv1.SecretKeySelector{})

	g.Expect(instance.Status.PrivateKeyPasswordRef).To(BeNil())
}

func TestCTlogKeys_PKCS11DisablesGenerateSigner(t *testing.T) {
	g := NewWithT(t)
	instance := ctlogInstance()
	instance.Spec.Logs[0].Signer.Type = rhtasv1.SignerTypePKCS11

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestCTlogKeys_FileModeEnablesGenerateSigner(t *testing.T) {
	g := NewWithT(t)
	instance := ctlogInstance()
	instance.Spec.Logs[0].Signer.Type = rhtasv1.SignerTypeFile

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestCTlogKeys_EmptyTypeEnablesGenerateSigner(t *testing.T) {
	g := NewWithT(t)
	instance := ctlogInstance()
	instance.Spec.Logs[0].Signer.Type = ""

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestCTlogKeys_UnencryptedKeyAllowedInFIPS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	g.Expect(err).ToNot(HaveOccurred())
	keyBytes, err := x509.MarshalECPrivateKey(key)
	g.Expect(err).ToNot(HaveOccurred())
	unencryptedKey := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	instance := ctlogInstance()
	instance.Spec.Logs[0].Signer.File = &rhtasv1.CTlogFile{
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
			Key:                  "private",
		},
	}

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-secret", Namespace: "default"},
		Data:       map[string][]byte{"private": unencryptedKey},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, userSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewFIPSValidationAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
}
