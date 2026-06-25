package actions

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func fulcioInstance() *rhtasv1.Fulcio {
	return &rhtasv1.Fulcio{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance",
			Namespace: "default",
		},
		Status: rhtasv1.FulcioStatus{
			Conditions: []metav1.Condition{
				{Type: constants.ReadyCondition, Status: metav1.ConditionFalse, Reason: state.Pending.String()},
			},
		},
	}
}

func TestFulcioCert_IsAlwaysEnabled(t *testing.T) {
	g := NewWithT(t)
	instance := fulcioInstance()

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestFulcioCert_UserProvidedCert(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	g.Expect(err).ToNot(HaveOccurred())
	pemKey, err := utils.CreateCAKey(key)
	g.Expect(err).ToNot(HaveOccurred())

	instance := fulcioInstance()
	instance.Spec.Certificate = rhtasv1.FulcioCert{
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-private"},
			Key:                  "private",
		},
		CARef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-cert"},
			Key:                  "cert",
		},
		OrganizationName:  "RH",
		OrganizationEmail: "jdoe@redhat.com",
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "user-private", Namespace: "default"},
				Data:       map[string][]byte{"private": pemKey},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "user-cert", Namespace: "default"},
				Data:       map[string][]byte{"cert": []byte("fake-cert")},
			},
		).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Certificate).ToNot(BeNil())
	g.Expect(instance.Status.Certificate.PrivateKeyRef.Name).To(Equal("user-private"))
	g.Expect(instance.Status.Certificate.CARef.Name).To(Equal("user-cert"))
	g.Expect(instance.Status.Certificate.CommonName).ToNot(BeEmpty())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, CertCondition)).To(BeTrue())
}

func TestFulcioCert_GeneratesCorrectData(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := fulcioInstance()
	instance.Spec.Certificate = rhtasv1.FulcioCert{
		OrganizationName:  "RH",
		OrganizationEmail: "jdoe@redhat.com",
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Certificate).ToNot(BeNil())
	g.Expect(instance.Status.Certificate.PrivateKeyRef).ToNot(BeNil())
	g.Expect(instance.Status.Certificate.CARef).ToNot(BeNil())
	g.Expect(instance.Status.Certificate.CommonName).ToNot(BeEmpty())

	secretName := fmt.Sprintf(certSecretNameFormat, "instance")
	g.Expect(instance.Status.Certificate.CARef.Name).To(Equal(secretName))

	secret := &corev1.Secret{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"},
	}), secret)).To(Succeed())

	g.Expect(secret.Data).To(HaveKey(constants.KeyPrivate))
	g.Expect(secret.Data).To(HaveKey(constants.KeyPublic))
	g.Expect(secret.Data).To(HaveKey(constants.KeyCert))
	g.Expect(secret.Data[constants.KeyPrivate]).To(ContainSubstring("EC PRIVATE KEY"))
	g.Expect(secret.Data[constants.KeyPublic]).To(ContainSubstring("PUBLIC KEY"))
	g.Expect(secret.Data[constants.KeyCert]).To(ContainSubstring("CERTIFICATE"))
	g.Expect(secret.Annotations).To(HaveKey(annotations.DataHash))
	g.Expect(secret.Labels).To(HaveKeyWithValue(FulcioCALabel, constants.KeyCert))
}

func TestFulcioCert_MigrationFromPreExistingSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := fulcioInstance()
	// Upgrade from <1.5.0: status references an old GenerateName-based secret
	instance.Status.Certificate = &rhtasv1.FulcioCertStatus{
		PrivateKeyRef: &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyPrivate,
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-cert-instance-abc12"},
		},
		CARef: &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyCert,
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "fulcio-cert-instance-abc12"},
		},
	}

	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "fulcio-cert-instance-abc12", Namespace: "default"},
		Data: map[string][]byte{
			constants.KeyPrivate: []byte("old-key"),
			constants.KeyCert:    []byte("old-cert"),
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
	g.Expect(instance.Status.Certificate).ToNot(BeNil())
	g.Expect(instance.Status.Certificate.CARef.Name).To(Equal("fulcio-cert-instance-abc12"))

	// No new deterministic-named secret should have been created
	newSecret := &corev1.Secret{}
	err := c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(certSecretNameFormat, "instance"), Namespace: "default"},
	}), newSecret)
	g.Expect(err).To(HaveOccurred())
}

func TestFulcioCert_DeterministicName(t *testing.T) {
	g := NewWithT(t)
	g.Expect(fmt.Sprintf(certSecretNameFormat, "my-fulcio")).To(Equal("fulcio-cert-config-my-fulcio"))
}

func TestFulcioCert_MissingPrivateKeyForCA(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := fulcioInstance()
	instance.Spec.Certificate = rhtasv1.FulcioCert{
		CARef: &rhtasv1.SecretKeySelector{
			LocalObjectReference: rhtasv1.LocalObjectReference{Name: "user-cert"},
			Key:                  "cert",
		},
		OrganizationName:  "RH",
		OrganizationEmail: "jdoe@redhat.com",
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		WithObjects(
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "user-cert", Namespace: "default"},
				Data:       map[string][]byte{"cert": []byte("fake-cert")},
			},
		).
		Build()

	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	result := a.Handle(ctx, instance)

	// Should fail with a condition indicating the error
	g.Expect(result).ToNot(BeNil())
	g.Expect(meta.IsStatusConditionFalse(instance.Status.Conditions, CertCondition)).To(BeTrue())
}

func TestFulcioCert_CanHandle_Resolved(t *testing.T) {
	g := NewWithT(t)
	instance := fulcioInstance()
	instance.Generation = 3
	instance.Status.Certificate = &rhtasv1.FulcioCertStatus{}
	instance.SetCondition(metav1.Condition{
		Type:               CertCondition,
		Reason:             "Resolved",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 3,
	})

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestFulcioCert_CanHandle_GenerationBump(t *testing.T) {
	g := NewWithT(t)
	instance := fulcioInstance()
	instance.Generation = 2
	instance.Status.Certificate = &rhtasv1.FulcioCertStatus{}
	instance.SetCondition(metav1.Condition{
		Type:               CertCondition,
		Reason:             "Resolved",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 1,
	})

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}
