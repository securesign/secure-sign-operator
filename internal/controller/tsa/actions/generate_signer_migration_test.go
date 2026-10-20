package actions

import (
	"errors"
	"maps"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// SECURESIGN-5702: on the way back the secret from the first file era still
// exists, so the pre-fix operator adopted it and resumed signing with the key
// the migration was meant to leave behind.
func TestTSASigner_FileKMSFileWithoutRefsFails(t *testing.T) {
	for _, fileType := range []string{"", rhtasv1.SignerTypeFile} {
		name := fileType
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()
			instance := tsaInstance()
			instance.Generation = 1
			instance.Spec.Signer.Type = fileType
			fileSigner := instance.Spec.Signer.DeepCopy()
			kmsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "kms-certificate", Namespace: instance.Namespace},
				Data:       map[string][]byte{tsaUtils.KeyCertificateChain: []byte("kms certificate")},
			}
			c := testAction.FakeClientBuilder().WithObjects(instance, kmsSecret).WithStatusSubresource(instance).Build()
			fileAction := testAction.PrepareAction(c, NewGenerateSignerAction())
			kmsAction := testAction.PrepareAction(c, NewResolveKMSTinkSignerAction())

			// First install: the operator generates once.
			g.Expect(fileAction.CanHandle(ctx, instance)).To(BeTrue())
			g.Expect(fileAction.Handle(ctx, instance)).To(Equal(testAction.Return()))
			firstEra := &corev1.Secret{}
			g.Expect(c.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: instance.Status.Signer.FileSigner.PrivateKeyRef.Name}, firstEra)).To(Succeed())
			g.Expect(firstEra.Data[tsaUtils.KeyLeafPrivateKey]).ToNot(BeEmpty())
			g.Expect(firstEra.Data[tsaUtils.KeyCertificateChain]).ToNot(BeEmpty())
			firstEraData := map[string][]byte{}
			maps.Copy(firstEraData, firstEra.Data)

			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
				Type: rhtasv1.SignerTypeKMS,
				Kms:  &rhtasv1.KMS{KeyResource: "awskms://test-key"},
				CertificateChain: rhtasv1.CertificateChain{
					CertificateChainRef: &rhtasv1.SecretKeySelector{
						LocalObjectReference: rhtasv1.LocalObjectReference{Name: kmsSecret.Name},
						Key:                  tsaUtils.KeyCertificateChain,
					},
				},
			}
			instance.Generation++
			g.Expect(c.Update(ctx, instance)).To(Succeed())
			g.Expect(kmsAction.CanHandle(ctx, instance)).To(BeTrue())
			g.Expect(kmsAction.Handle(ctx, instance)).To(Equal(testAction.Return()))
			g.Expect(instance.Status.Signer.CertificateChainRef.Name).To(Equal(kmsSecret.Name))
			g.Expect(instance.Status.Signer.FileSigner).To(BeNil())

			// Return to file mode by only dropping the KMS settings.
			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			instance.Spec.Signer = *fileSigner.DeepCopy()
			instance.Generation++
			g.Expect(c.Update(ctx, instance)).To(Succeed())

			g.Expect(fileAction.CanHandle(ctx, instance)).To(BeTrue())
			result := fileAction.Handle(ctx, instance)
			g.Expect(result).ToNot(BeNil())
			g.Expect(errors.Is(result.Err, ErrReturnToFileNeedsRef)).To(BeTrue())
			// Not terminal: BaseAction.Error would then force Ready=Failure, and because
			// state.Failure < state.Pending no action's CanHandle gate lets the component
			// run again — supplying the missing ref afterwards would never recover it.
			g.Expect(errors.Is(result.Err, reconcile.TerminalError(result.Err))).To(BeFalse())
			g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeFalse())

			g.Expect(instance.Status.Signer.FileSigner).To(BeNil())
			retained := &corev1.Secret{}
			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(firstEra), retained)).To(Succeed())
			g.Expect(retained.Data).To(Equal(firstEraData))
			secrets := &corev1.SecretList{}
			g.Expect(c.List(ctx, secrets, client.InNamespace(instance.Namespace))).To(Succeed())
			g.Expect(secrets.Items).To(HaveLen(2), "a refused return must not generate a signer")

			g.Expect(errors.Is(fileAction.Handle(ctx, instance).Err, ErrReturnToFileNeedsRef)).To(BeTrue())
			g.Expect(c.List(ctx, secrets, client.InNamespace(instance.Namespace))).To(Succeed())
			g.Expect(secrets.Items).To(HaveLen(2))
		})
	}
}

// Same guard with no earlier file era: the instance was initialised once
// already, so the operator does not generate again.
func TestTSASigner_KMSToFileWithoutRefsFails(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Generation = 3
	instance.Status.Signer = &rhtasv1.TimestampAuthoritySignerStatus{
		CertificateChainRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-kms-cert"}, Key: tsaUtils.KeyCertificateChain},
	}

	c := testAction.FakeClientBuilder().WithObjects(instance).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())

	result := a.Handle(ctx, instance)
	g.Expect(result).ToNot(BeNil())
	g.Expect(errors.Is(result.Err, ErrReturnToFileNeedsRef)).To(BeTrue())

	secrets := &corev1.SecretList{}
	g.Expect(c.List(ctx, secrets, client.InNamespace(instance.Namespace))).To(Succeed())
	g.Expect(secrets.Items).To(BeEmpty())
}

func TestTSASigner_KMSToFileUsesExplicitRefs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()
	instance := tsaInstance()
	instance.Generation = 3
	keyRef := &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "customer-key"}, Key: tsaUtils.KeyLeafPrivateKey}
	certRef := &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "customer-cert"}, Key: tsaUtils.KeyCertificateChain}
	instance.Spec.Signer = rhtasv1.TimestampAuthoritySigner{
		Type:             rhtasv1.SignerTypeFile,
		File:             &rhtasv1.File{PrivateKeyRef: *keyRef},
		CertificateChain: rhtasv1.CertificateChain{CertificateChainRef: certRef},
	}
	instance.Status.Signer = &rhtasv1.TimestampAuthoritySignerStatus{
		CertificateChainRef: &rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: "old-kms-cert"}, Key: tsaUtils.KeyCertificateChain},
	}
	keySecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: keyRef.Name, Namespace: instance.Namespace}, Data: map[string][]byte{keyRef.Key: []byte("customer key")}}
	certSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: certRef.Name, Namespace: instance.Namespace}, Data: map[string][]byte{certRef.Key: []byte("customer certificate")}}
	c := testAction.FakeClientBuilder().WithObjects(instance, keySecret, certSecret).WithStatusSubresource(instance).Build()
	a := testAction.PrepareAction(c, NewGenerateSignerAction())
	g.Expect(a.Handle(ctx, instance)).To(Equal(testAction.Return()))
	g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef).To(Equal(keyRef))
	g.Expect(instance.Status.Signer.CertificateChainRef).To(Equal(certRef))
	secrets := &corev1.SecretList{}
	g.Expect(c.List(ctx, secrets, client.InNamespace(instance.Namespace))).To(Succeed())
	g.Expect(secrets.Items).To(HaveLen(2), "explicit refs must not trigger automatic signer generation")
}
