package actions

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	testAction "github.com/securesign/operator/internal/testing/action"
	common "github.com/securesign/operator/internal/testing/common/tsa"
	"github.com/securesign/operator/test/e2e/support"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_NewGenerateSignerAction(t *testing.T) {
	g := NewWithT(t)

	action := NewGenerateSignerAction()
	g.Expect(action).ToNot(BeNil())
}

func Test_SignerName(t *testing.T) {
	g := NewWithT(t)

	action := NewGenerateSignerAction()
	g.Expect(action.Name()).To(Equal("handle certificate chain"))
}

func Test_SignerCanHandle(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		testCase func(*rhtasv1alpha1.TimestampAuthority)
		expected bool
	}{
		{
			name:     "Default condition",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {},
			expected: true,
		},
		{
			name: "Pending condition",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Status.Conditions[0].Reason = constants.Pending
			},
			expected: true,
		},
		{
			name: "status is not nil",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Status.Signer = &instance.Spec.Signer
			},
			expected: false,
		},
		{
			name: "spec and status differ",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Status.Signer = &rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "new_org",
						},
						IntermediateCA: []rhtasv1alpha1.TsaCertificateAuthority{
							{
								OrganizationName: "new_org",
							},
						},
						LeafCA: rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "new_org",
						},
					},
				}
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewGenerateSignerAction()
			instance := common.GenerateTSAInstance()
			tt.testCase(instance)
			g.Expect(action.CanHandle(context.TODO(), instance)).To(Equal(tt.expected))
		})
	}

}

func Test_SignerHandle(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority])
		testCase func(Gomega, action.Action[*rhtasv1alpha1.TimestampAuthority], client.WithWatch, *rhtasv1alpha1.TimestampAuthority) bool
	}{
		{
			name: "generate certificate keys and certs",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), []client.Object{}...)
			},
			testCase: func(g Gomega, _ action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {

				secret, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				if err != nil {
					t.Errorf("Unable to find secret: %s", err)
					return false
				}
				if !g.Expect(instance.Status.Signer).NotTo(BeNil()) {
					t.Error("Status Signer should not be nil")
					return false
				}
				if !g.Expect(secret.Name).To(Equal(instance.Status.Signer.CertificateChain.CertificateChainRef.Name)) {
					t.Errorf("Secret name mismatch: expected %v, got %v", instance.Status.Signer.CertificateChain.CertificateChainRef.Name, secret.Name)
					return false
				}
				if !g.Expect(secret.Name).To(Equal(instance.Status.Signer.File.PrivateKeyRef.Name)) {
					t.Errorf("Secret name mismatch: expected %v, got %v", instance.Status.Signer.File.PrivateKeyRef.Name, secret.Name)
					return false
				}
				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition).Reason).To(Equal("Resolved"))
			},
		},
		{
			name: "generate certs",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "Red Hat",
							PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
								LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
									Name: "tsa-test-secret",
								},
								Key: "rootPrivateKey",
							},
							PasswordRef: &rhtasv1alpha1.SecretKeySelector{
								LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
									Name: "tsa-test-secret",
								},
								Key: "rootPrivateKeyPassword",
							},
						},
						IntermediateCA: []rhtasv1alpha1.TsaCertificateAuthority{
							{
								OrganizationName: "Red Hat",
								PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
									LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
										Name: "tsa-test-secret",
									},
									Key: "interPrivateKey-0",
								},
								PasswordRef: &rhtasv1alpha1.SecretKeySelector{
									LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
										Name: "tsa-test-secret",
									},
									Key: "interPrivateKeyPassword-0",
								},
							},
						},
						LeafCA: rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "Red Hat",
							PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
								LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
									Name: "tsa-test-secret",
								},
								Key: "leafPrivateKey",
							},
							PasswordRef: &rhtasv1alpha1.SecretKeySelector{
								LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
									Name: "tsa-test-secret",
								},
								Key: "leafPrivateKeyPassword",
							},
						},
					},
				}

				obj := []client.Object{}
				client := testAction.FakeClientBuilder().WithObjects(instance).Build()
				secret := support.InitTsaSecrets(instance.Namespace, "tsa-test-secret")
				obj = append(obj, secret)
				return common.TsaTestSetup(instance, t, client, NewGenerateSignerAction(), obj...)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {

				secret, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				if err != nil {
					t.Errorf("Unable to find secret: %s", err)
					return false
				}
				if !g.Expect(instance.Status.Signer).NotTo(BeNil()) {
					t.Error("Status Signer should not be nil")
					return false
				}
				if !g.Expect(secret.Name).To(Equal(instance.Status.Signer.CertificateChain.CertificateChainRef.Name)) {
					t.Errorf("Secret name mismatch: expected %v, got %v", instance.Status.Signer.CertificateChain.CertificateChainRef.Name, secret.Name)
					return false
				}
				if !g.Expect(secret.Name).To(Equal(instance.Status.Signer.File.PrivateKeyRef.Name)) {
					t.Errorf("Secret name mismatch: expected %v, got %v", instance.Status.Signer.File.PrivateKeyRef.Name, secret.Name)
					return false
				}
				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition).Reason).To(Equal("Resolved"))
			},
		},
		{
			name: "predefined keys and certs",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						CertificateChainRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "certificateChain",
						},
					},
					File: &rhtasv1alpha1.File{
						PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "leafPrivateKey",
						},
						PasswordRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "leafPrivateKeyPassword",
						},
					},
				}
				obj := []client.Object{}
				client := testAction.FakeClientBuilder().WithObjects(instance).Build()
				secret := support.InitTsaSecrets(instance.Namespace, "tsa-test-secret")
				obj = append(obj, secret)
				return common.TsaTestSetup(instance, t, client, NewGenerateSignerAction(), obj...)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				_, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				if err != nil {
					t.Errorf("Unable to find secret: %s", err)
					return false
				}

				if !g.Expect(instance.Status.Signer).NotTo(BeNil()) {
					t.Error("Status Signer should not be nil")
					return false
				}

				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition).Reason).To(Equal("Resolved"))
			},
		},
		{
			name: "should update secret resource",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				client := testAction.FakeClientBuilder().WithObjects(instance).Build()
				secret := support.InitTsaSecrets(instance.Namespace, "tsa-test-secret")
				return common.TsaTestSetup(instance, t, client, NewGenerateSignerAction(), secret)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				secret, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				if err != nil {
					t.Errorf("Failed to find secret: %v", err)
					return false
				}
				if !g.Expect(instance.Status.Signer).NotTo(BeNil()) {
					t.Error("Signer should not be nil")
					return false
				}
				if !g.Expect(secret.Name).To(Equal(instance.Status.Signer.CertificateChain.CertificateChainRef.Name)) {
					t.Errorf("Secret name mismatch: expected %v, got %v", instance.Status.Signer.CertificateChain.CertificateChainRef.Name, secret.Name)
					return false
				}
				if !g.Expect(secret.Name).To(Equal(instance.Status.Signer.File.PrivateKeyRef.Name)) {
					t.Errorf("Private key ref name mismatch: expected %v, got %v", instance.Status.Signer.File.PrivateKeyRef.Name, secret.Name)
					return false
				}
				oldSecretName := secret.Name
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						CertificateChainRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "certificateChain",
						},
					},
					File: &rhtasv1alpha1.File{
						PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "leafPrivateKey",
						},
						PasswordRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "leafPrivateKeyPassword",
						},
					},
				}

				if err := client.Update(context.TODO(), instance); err != nil {
					t.Errorf("Failed to update instance: %v", err)
					return false
				}
				_ = a.Handle(context.TODO(), instance)
				secret, err = kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				if err != nil {
					t.Errorf("Failed to find updated secret: %v", err)
					return false
				}
				if !g.Expect(secret.Name).ToNot(Equal(oldSecretName)) {
					t.Errorf("Secret name should have changed: old=%v, new=%v", oldSecretName, secret.Name)
					return false
				}
				return g.Expect(meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition).Reason).To(Equal("Resolved"))
			},
		},
		{
			name: "should remove label after new secret creation",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				client := testAction.FakeClientBuilder().WithObjects(instance).Build()
				secret := support.InitTsaSecrets(instance.Namespace, "tsa-test-secret")
				return common.TsaTestSetup(instance, t, client, NewGenerateSignerAction(), secret)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				secret, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				if err != nil {
					t.Errorf("Failed to find secret: %v", err)
					return false
				}
				if _, exists := secret.Labels["rhtas.redhat.com/tsa.certchain.pem"]; !exists {
					t.Error("Label 'rhtas.redhat.com/tsa.certchain.pem' is not present in secret")
					return false
				}
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						CertificateChainRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "certificateChain",
						},
					},
					File: &rhtasv1alpha1.File{
						PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "leafPrivateKey",
						},
						PasswordRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-test-secret",
							},
							Key: "leafPrivateKeyPassword",
						},
					},
				}

				if err := client.Update(context.TODO(), instance); err != nil {
					t.Errorf("Failed to update instance: %v", err)
					return false
				}
				_ = a.Handle(context.TODO(), instance)

				oldSecret, err := kubernetes.GetSecret(client, instance.Namespace, secret.Name)
				if err != nil {
					t.Errorf("Failed to get old secret: %v", err)
					return false
				}

				if _, exists := oldSecret.Labels["rhtas.redhat.com/tsa.certchain.pem"]; exists {
					t.Error("Label 'rhtas.redhat.com/tsa.certchain.pem' should have been removed")
					return false
				}

				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := common.GenerateTSAInstance()
			client, action := tt.setup(instance)
			g.Expect(client).NotTo(BeNil())
			g.Expect(action).NotTo(BeNil())
			g.Expect(tt.testCase(g, action, client, instance)).To(BeTrue())
		})
	}
}
