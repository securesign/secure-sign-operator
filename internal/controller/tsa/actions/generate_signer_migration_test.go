package actions

import (
	"crypto/sha256"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	testAction "github.com/securesign/operator/internal/testing/action"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestTSASigner_FileKMSFileRotatesGeneratedMaterial(t *testing.T) {
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

			g.Expect(fileAction.CanHandle(ctx, instance)).To(BeTrue())
			g.Expect(fileAction.Handle(ctx, instance)).To(Equal(testAction.Return()))
			initial := &corev1.Secret{}
			g.Expect(c.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: instance.Status.Signer.FileSigner.PrivateKeyRef.Name}, initial)).To(Succeed())
			g.Expect(initial.Data[tsaUtils.KeyLeafPrivateKey]).ToNot(BeEmpty())
			g.Expect(initial.Data[tsaUtils.KeyCertificateChain]).ToNot(BeEmpty())
			history := make([]*corev1.Secret, 1, 3)
			history[0] = initial

			// Repeat the round trip to ensure each return rotates the current
			// file signer while preserving every historical key and certificate.
			for range 2 {
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

				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				instance.Spec.Signer = *fileSigner.DeepCopy()
				instance.Generation++
				g.Expect(c.Update(ctx, instance)).To(Succeed())
				kmsStatus := instance.Status.Signer.DeepCopy()
				kmsConditions := append([]metav1.Condition(nil), instance.Status.Conditions...)
				g.Expect(fileAction.CanHandle(ctx, instance)).To(BeTrue())
				g.Expect(fileAction.Handle(ctx, instance)).To(Equal(testAction.Return()))
				g.Expect(instance.Status.Signer.CertificateChainRef.Name).ToNot(Equal(kmsSecret.Name))
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal(instance.Status.Signer.CertificateChainRef.Name))

				active := &corev1.Secret{}
				g.Expect(c.Get(ctx, client.ObjectKey{Namespace: instance.Namespace, Name: instance.Status.Signer.FileSigner.PrivateKeyRef.Name}, active)).To(Succeed())
				g.Expect(active.Data[instance.Status.Signer.FileSigner.PrivateKeyRef.Key]).ToNot(BeEmpty())
				g.Expect(active.Data[instance.Status.Signer.CertificateChainRef.Key]).ToNot(BeEmpty())
				for _, previous := range history {
					g.Expect(active.Name).ToNot(Equal(previous.Name), "must not reactivate a historical File signer")
					g.Expect(sha256.Sum256(active.Data[tsaUtils.KeyLeafPrivateKey])).ToNot(Equal(sha256.Sum256(previous.Data[tsaUtils.KeyLeafPrivateKey])), "must generate a fresh private key")
					g.Expect(sha256.Sum256(active.Data[tsaUtils.KeyCertificateChain])).ToNot(Equal(sha256.Sum256(previous.Data[tsaUtils.KeyCertificateChain])), "must generate a fresh certificate")
					retained := &corev1.Secret{}
					g.Expect(c.Get(ctx, client.ObjectKeyFromObject(previous), retained)).To(Succeed())
					g.Expect(retained.Data).To(Equal(previous.Data), "historical signer material must remain unchanged")
				}
				history = append(history, active.DeepCopy())

				// Simulate a restart after Secret creation but before status
				// persistence. The retry must select the same generated Secret.
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				instance.Status.Signer = kmsStatus
				instance.Status.Conditions = kmsConditions
				g.Expect(c.Status().Update(ctx, instance)).To(Succeed())
				fileAction = testAction.PrepareAction(c, NewGenerateSignerAction())
				g.Expect(fileAction.Handle(ctx, instance)).To(Equal(testAction.Return()))
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal(active.Name))

				// An unrelated spec update must retain the newly selected key.
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				instance.Spec.Signer.CertificateChain.RootCA.OrganizationName = "Updated organization"
				instance.Generation++
				g.Expect(c.Update(ctx, instance)).To(Succeed())
				g.Expect(fileAction.CanHandle(ctx, instance)).To(BeTrue())
				g.Expect(fileAction.Handle(ctx, instance)).To(Equal(testAction.Return()))
				g.Expect(instance.Status.Signer.FileSigner.PrivateKeyRef.Name).To(Equal(active.Name))
				unchanged := &corev1.Secret{}
				g.Expect(c.Get(ctx, client.ObjectKeyFromObject(active), unchanged)).To(Succeed())
				g.Expect(unchanged.Data).To(Equal(active.Data))
				secrets := &corev1.SecretList{}
				g.Expect(c.List(ctx, secrets, client.InNamespace(instance.Namespace))).To(Succeed())
				g.Expect(secrets.Items).To(HaveLen(len(history)+1), "retries must not create additional Secrets")
			}
		})
	}
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
