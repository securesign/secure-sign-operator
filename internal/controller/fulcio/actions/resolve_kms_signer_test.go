package actions

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestResolveKMSSigner_HappyPath(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := fulcioInstance()
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.FulcioCertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "kms-cert-chain"},
				Key:                  "cert",
			},
		},
		Kms: &rhtasv1.KMS{KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k"},
	}

	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "kms-cert-chain", Namespace: "default"},
		Data:       map[string][]byte{"cert": []byte("cert-chain-data")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, certSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())

	result := a.Handle(ctx, instance)
	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Certificate).NotTo(BeNil())
	g.Expect(instance.Status.Certificate.CARef).NotTo(BeNil())
	g.Expect(instance.Status.Certificate.CARef.Name).To(Equal("kms-cert-chain"))
	g.Expect(instance.Status.Certificate.PrivateKeyRef).To(BeNil())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, CertCondition)).To(BeTrue())
}

func TestResolveKMSSigner_DisabledForFileType(t *testing.T) {
	g := NewWithT(t)
	instance := fulcioInstance()
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeFile,
	}

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestResolveKMSSigner_MissingCertChainSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := fulcioInstance()
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.FulcioCertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "missing-secret"},
				Key:                  "cert",
			},
		},
		Kms: &rhtasv1.KMS{KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k"},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())

	result := a.Handle(ctx, instance)
	g.Expect(result).NotTo(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(result.Err.Error()).To(ContainSubstring("not found"))
}

func TestResolveKMSSigner_ConditionAlreadySatisfied(t *testing.T) {
	g := NewWithT(t)
	instance := fulcioInstance()
	instance.Generation = 1
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.FulcioCertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "cert-chain"},
				Key:                  "cert",
			},
		},
		Kms: &rhtasv1.KMS{KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k"},
	}
	instance.SetCondition(metav1.Condition{
		Type:               CertCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "Resolved",
		ObservedGeneration: 1,
	})

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestResolveKMSSigner_GenerationBump(t *testing.T) {
	g := NewWithT(t)
	instance := fulcioInstance()
	instance.Generation = 2
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.FulcioCertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "cert-chain"},
				Key:                  "cert",
			},
		},
		Kms: &rhtasv1.KMS{KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k"},
	}
	instance.SetCondition(metav1.Condition{
		Type:               CertCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "Resolved",
		ObservedGeneration: 1,
	})

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeTrue())
}

func TestResolveKMSSigner_CanHandleNoReadyCondition(t *testing.T) {
	g := NewWithT(t)
	instance := fulcioInstance()
	instance.Status.Conditions = nil
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeKMS,
		Kms:  &rhtasv1.KMS{KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k"},
	}

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestResolveKMSSigner_MissingSecret_SetsConditions(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := fulcioInstance()
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.FulcioCertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "missing-secret"},
				Key:                  "cert",
			},
		},
		Kms: &rhtasv1.KMS{KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k"},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	_ = a.Handle(ctx, instance)

	certCond := meta.FindStatusCondition(instance.Status.Conditions, CertCondition)
	g.Expect(certCond).NotTo(BeNil())
	g.Expect(certCond.Status).To(Equal(metav1.ConditionFalse))
}

func TestResolveKMSSigner_SetsCALabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := fulcioInstance()
	instance.Spec.Signer = rhtasv1.FulcioSigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.FulcioCertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "kms-cert-chain"},
				Key:                  "cert",
			},
		},
		Kms: &rhtasv1.KMS{KeyResource: "gcpkms://projects/p/locations/l/keyRings/kr/cryptoKeys/k"},
	}

	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "kms-cert-chain", Namespace: "default"},
		Data:       map[string][]byte{"cert": []byte("cert-chain-data")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, certSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).To(Equal(testAction.Return()))

	updatedSecret := &corev1.Secret{}
	g.Expect(c.Get(ctx, client.ObjectKeyFromObject(certSecret), updatedSecret)).To(Succeed())
	g.Expect(updatedSecret.Labels).To(HaveKeyWithValue(FulcioCALabel, "cert"))
}
