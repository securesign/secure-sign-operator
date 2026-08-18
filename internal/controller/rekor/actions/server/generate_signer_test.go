package server

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"github.com/securesign/operator/internal/utils/fips"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

func rekorInstance() *rhtasv1.Rekor {
	return &rhtasv1.Rekor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rekor",
			Namespace: "default",
		},
		Status: rhtasv1.RekorStatus{
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
				{Type: actions.SignerCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
			},
		},
	}
}

func TestRekorSigner_KMSDisabled(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()
	instance.Spec.Signer.Type = rhtasv1.RekorSignerTypeKMS
	instance.Spec.Signer.Kms = &rhtasv1.KMS{KeyResource: "awskms://key"}

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestRekorSigner_KMSSecretEnabled(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()
	instance.Spec.Signer.Type = rhtasv1.RekorSignerTypeSecret

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestRekorSigner_EmptyKMSEnabled(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestRekorSigner_UserProvidedKeyRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := rekorInstance()
	instance.Spec.Signer.KeyRef = &rhtasv1.SecretKeySelector{
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
	g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("user-secret"))
	g.Expect(instance.Status.PublicKeyRef).To(BeNil())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition)).To(BeTrue())
}

func TestRekorSigner_GeneratesCorrectKeyData(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := rekorInstance()

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Signer.KeyRef).ToNot(BeNil())
	g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("rekor-signer-config-rekor"))
	g.Expect(instance.Status.Signer.KeyRef.Key).To(Equal(constants.KeyPrivate))
	g.Expect(instance.Status.PublicKeyRef).To(BeNil())

	secret := &corev1.Secret{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rekor-signer-config-rekor", Namespace: "default"},
	}), secret)).To(Succeed())

	g.Expect(secret.Data).To(HaveKey(constants.KeyPrivate))
	g.Expect(secret.Data).To(HaveKey(constants.KeyPublic))
	g.Expect(secret.Data[constants.KeyPrivate]).To(ContainSubstring("EC PRIVATE KEY"))
	g.Expect(secret.Data[constants.KeyPublic]).To(ContainSubstring("PUBLIC KEY"))
	g.Expect(secret.Labels).ToNot(BeEmpty())
}

func TestRekorSigner_MigrationFromPreExistingSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := rekorInstance()
	// Upgrade from <1.5.0: status references an old GenerateName-based secret
	instance.Status.Signer.KeyRef = &rhtasv1.SecretKeySelector{
		Key:                  constants.KeyPrivate,
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "rekor-signer-rekor-7k2x9"},
	}

	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "rekor-signer-rekor-7k2x9", Namespace: "default"},
		Data:       map[string][]byte{constants.KeyPrivate: []byte("old-key")},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance, oldSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	// Status should still reference the OLD secret
	g.Expect(instance.Status.Signer.KeyRef).ToNot(BeNil())
	g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("rekor-signer-rekor-7k2x9"))
	g.Expect(instance.Status.PublicKeyRef).To(BeNil())

	// No new deterministic-named secret should have been created
	newSecret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(signerSecretNameFormat, "rekor"), Namespace: "default"},
	}), newSecret)
	g.Expect(err).To(HaveOccurred())
}

func TestRekorSigner_KeyRefChangePreservesCachedPublicKey(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := rekorInstance()
	instance.Status.PublicKey = "-----BEGIN PUBLIC KEY-----\nOLDKEY\n-----END PUBLIC KEY-----\n"
	instance.Spec.Signer.KeyRef = &rhtasv1.SecretKeySelector{
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
	a.Handle(ctx, instance)

	g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("user-secret"))
	g.Expect(instance.Status.PublicKey).To(Equal("-----BEGIN PUBLIC KEY-----\nOLDKEY\n-----END PUBLIC KEY-----\n"),
		"realigning the signer secret ref must not clobber the cached public key — trustmaterial owns that field and needs the prior value to detect drift")
}

func TestRekorSigner_DeterministicName(t *testing.T) {
	g := NewWithT(t)
	g.Expect(fmt.Sprintf(signerSecretNameFormat, "my-rekor")).To(Equal("rekor-signer-config-my-rekor"))
}

func TestRekorSigner_AlignStatusPreservesPasswordRefWhenKeyRefUnchanged(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()

	// Pre-existing status with both KeyRef and PasswordRef set (from older API version)
	instance.Status.Signer = rhtasv1.RekorSignerStatus{
		KeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
			Key:                  "private",
		},
		PasswordRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "password-secret"},
			Key:                  "password",
		},
	}

	// Spec points to the SAME key
	instance.Spec.Signer.KeyRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-secret"},
		Key:                  "private",
	}

	ref := rhtasv1.SecretKeySelector{}
	alignStatus(instance, ref)

	g.Expect(instance.Status.Signer.KeyRef).ToNot(BeNil())
	g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("user-secret"))
	g.Expect(instance.Status.Signer.PasswordRef).ToNot(BeNil(),
		"PasswordRef must be preserved when KeyRef has not changed")
	g.Expect(instance.Status.Signer.PasswordRef.Name).To(Equal("password-secret"))
	g.Expect(instance.Status.Signer.PasswordRef.Key).To(Equal("password"))
}

func TestRekorSigner_AlignStatusDropsPasswordRefWhenKeyRefChanges(t *testing.T) {
	g := NewWithT(t)
	instance := rekorInstance()

	// Pre-existing status with both KeyRef and PasswordRef set (from older API version)
	instance.Status.Signer = rhtasv1.RekorSignerStatus{
		KeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-secret"},
			Key:                  "private",
		},
		PasswordRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "password-secret"},
			Key:                  "password",
		},
	}

	// Spec points to a DIFFERENT key
	instance.Spec.Signer.KeyRef = &rhtasv1.SecretKeySelector{
		LocalObjectReference: rhtasv1.LocalObjectReference{Name: "new-secret"},
		Key:                  "private",
	}

	ref := rhtasv1.SecretKeySelector{}
	alignStatus(instance, ref)

	g.Expect(instance.Status.Signer.KeyRef).ToNot(BeNil())
	g.Expect(instance.Status.Signer.KeyRef.Name).To(Equal("new-secret"))
	g.Expect(instance.Status.Signer.PasswordRef).To(BeNil(),
		"PasswordRef must be dropped when KeyRef has changed")
}

func TestRekorSigner_UnencryptedKeyAllowedInFIPS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	original := fips.Enabled
	fips.Enabled = func() bool { return true }
	t.Cleanup(func() { fips.Enabled = original })

	unencryptedKey, _, err := createSignerKey()
	g.Expect(err).ToNot(HaveOccurred())

	instance := rekorInstance()
	instance.Spec.Signer.KeyRef = &rhtasv1.SecretKeySelector{
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

	a := testAction.PrepareAction(c, NewFIPSValidationAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
}
