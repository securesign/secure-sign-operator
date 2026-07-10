package actions

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func ctlogInstance() *rhtasv1.CTlog {
	return &rhtasv1.CTlog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance",
			Namespace: "default",
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
	instance.Spec.PrivateKeyRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "private",
	}
	instance.Spec.PublicKeyRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "public",
	}
	instance.Spec.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{ //nolint:staticcheck
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "password",
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
	g.Expect(instance.Status.PrivateKeyPasswordRef.Name).To(Equal("user-secret")) //nolint:staticcheck
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
	instance.Spec.PrivateKeyRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "private",
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

func TestCTlogKeys_PasswordRefRejectedInFIPS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	instance := ctlogInstance()
	instance.Spec.PrivateKeyRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "private",
	}
	instance.Spec.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{ //nolint:staticcheck
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-password"},
		Key:                  "password",
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
	g.Expect(result.Err.Error()).To(ContainSubstring("FIPS"))
	cond := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
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
	instance.Spec.PrivateKeyRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "private",
	}

	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "user-secret", Namespace: "default"},
		Data:       map[string][]byte{"private": unencryptedKey},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, userSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result.Err).ToNot(HaveOccurred())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, SignerCondition)).To(BeTrue())
}
