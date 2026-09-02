package actions

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	generateSigner "github.com/securesign/operator/internal/action/generateSigner"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestKMSTinkSigner_KMSWithCertChainRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "kms-cert-secret"},
				Key:                  "certificateChain",
			},
		},
		Kms: &rhtasv1.KMS{
			KeyResource: "projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key/cryptoKeyVersions/1",
		},
	}

	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "kms-cert-secret", Namespace: "default"},
		Data:       map[string][]byte{"certificateChain": []byte("cert-chain-data")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, certSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSTinkSignerAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())

	result := a.Handle(ctx, instance)
	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Signer).NotTo(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef).NotTo(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("kms-cert-secret"))
	g.Expect(instance.Status.Signer.FileSigner).To(BeNil())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())
}

func TestKMSTinkSigner_TinkWithCertChainRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		Type: rhtasv1.SignerTypeTink,
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tink-cert-secret"},
				Key:                  "certificateChain",
			},
		},
		Tink: &rhtasv1.Tink{
			KeysetRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tink-keyset-secret"},
				Key:                  "keySet",
			},
		},
	}

	certSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tink-cert-secret", Namespace: "default"},
		Data:       map[string][]byte{"certificateChain": []byte("cert-chain-data")},
	}
	keysetSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tink-keyset-secret", Namespace: "default"},
		Data:       map[string][]byte{"keySet": []byte("keyset-data")},
	}
	c := testAction.FakeClientBuilder().
		WithObjects(instance, certSecret, keysetSecret).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSTinkSignerAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())

	result := a.Handle(ctx, instance)
	g.Expect(result).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Signer).NotTo(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef).NotTo(BeNil())
	g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal("tink-cert-secret"))
	g.Expect(instance.Status.Signer.FileSigner).To(BeNil())
	g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())
}

func TestKMSTinkSigner_TinkWithoutKeysetRef(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		Type: rhtasv1.SignerTypeTink,
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "tink-cert-secret"},
				Key:                  "certificateChain",
			},
		},
		Tink: &rhtasv1.Tink{},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSTinkSignerAction())
	g.Expect(a.CanHandle(ctx, instance)).To(BeTrue())

	result := a.Handle(ctx, instance)
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeTrue())
	g.Expect(result.Err.Error()).To(ContainSubstring("missing keyset reference"))

	cond := meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition)
	g.Expect(cond).NotTo(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
}

func TestKMSTinkSigner_FileTypeDisabled(t *testing.T) {
	g := NewWithT(t)
	instance := tsaInstance()

	c := testAction.FakeClientBuilder().Build()
	a := testAction.PrepareAction(c, NewResolveKMSTinkSignerAction())
	g.Expect(a.CanHandle(t.Context(), instance)).To(BeFalse())
}

func TestKMSTinkSigner_MissingCertChainSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		Type: rhtasv1.SignerTypeKMS,
		CertificateChain: rhtasv1.CertificateChain{
			CertificateChainRef: &rhtasv1.SecretKeySelector{
				LocalObjectReference: rhtasv1.LocalObjectReference{Name: "missing-secret"},
				Key:                  "certificateChain",
			},
		},
		Kms: &rhtasv1.KMS{
			KeyResource: "projects/my-project/locations/global/keyRings/my-ring/cryptoKeys/my-key/cryptoKeyVersions/1",
		},
	}

	c := testAction.FakeClientBuilder().
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	a := testAction.PrepareAction(c, NewResolveKMSTinkSignerAction())
	result := a.Handle(ctx, instance)

	g.Expect(result).NotTo(BeNil())
	g.Expect(result.Err).To(HaveOccurred())
	g.Expect(errors.Is(result.Err, generateSigner.ErrSecretNotFound)).To(BeTrue())
}
