package actions

import (
	"context"
	"crypto/elliptic"
	"encoding/json"
	"testing"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	cryptoutil "github.com/securesign/operator/internal/utils/crypto"
	fipsTest "github.com/securesign/operator/internal/utils/crypto/test"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
	v1 "k8s.io/api/core/v1"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	common "github.com/securesign/operator/internal/testing/common/tsa"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				instance.Status.Conditions = []metav1.Condition{
					{
						Type:   "TSASignerCondition",
						Status: metav1.ConditionTrue,
						Reason: constants.Ready,
					},
				}
				instance.Status.Signer = &instance.Spec.Signer
			},
			expected: false,
		},
		{
			name: "spec and status differ",
			testCase: func(instance *rhtasv1alpha1.TimestampAuthority) {
				instance.Status.Signer = &rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: &rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "new_org",
						},
						IntermediateCA: []*rhtasv1alpha1.TsaCertificateAuthority{
							{
								OrganizationName: "new_org",
							},
						},
						LeafCA: &rhtasv1alpha1.TsaCertificateAuthority{
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
			name: "generate all resources",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), []client.Object{}...)
			},
			testCase: func(g Gomega, _ action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {

				secret, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				g.Expect(err).NotTo(HaveOccurred(), "Unable to find secret")

				g.Expect(instance.Status.Signer).NotTo(BeNil(), "Status Signer should not be nil")

				g.Expect(secret.Name).To(Equal(instance.Status.Signer.CertificateChain.CertificateChainRef.Name), "Secret name mismatch for CertificateChainRef")
				g.Expect(secret.Name).To(Equal(instance.Status.Signer.File.PrivateKeyRef.Name), "Secret name mismatch for PrivateKeyRef")
				g.Expect(secret.Annotations).To(Equal(generateSecretAnnotations(instance.Spec.Signer)), "Secret annotation mismatch")

				g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())

				return true
			},
		},
		{
			name: "generate certs with user-specified keys",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: &rhtasv1alpha1.TsaCertificateAuthority{
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
						IntermediateCA: []*rhtasv1alpha1.TsaCertificateAuthority{
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
						LeafCA: &rhtasv1alpha1.TsaCertificateAuthority{
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

				secret := tsa.CreateSecrets(instance.Namespace, "tsa-test-secret")
				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), secret)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {

				secret, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				g.Expect(err).NotTo(HaveOccurred(), "Unable to find secret")

				g.Expect(instance.Status.Signer).NotTo(BeNil(), "Status Signer should not be nil")

				g.Expect(secret.Name).To(Equal(instance.Status.Signer.CertificateChain.CertificateChainRef.Name), "Secret name mismatch for CertificateChainRef")
				g.Expect(secret.Name).To(Equal(instance.Status.Signer.File.PrivateKeyRef.Name), "Secret name mismatch for PrivateKeyRef")

				g.Expect(instance.Status.Signer.CertificateChain.RootCA.PrivateKeyRef.Name).To(Equal("tsa-test-secret"))
				g.Expect(instance.Status.Signer.CertificateChain.LeafCA.PrivateKeyRef.Name).To(Equal("tsa-test-secret"))
				g.Expect(instance.Status.Signer.CertificateChain.IntermediateCA[0].PrivateKeyRef.Name).To(Equal("tsa-test-secret"))

				g.Expect(secret.Annotations).To(Equal(generateSecretAnnotations(instance.Spec.Signer)), "Secret annotation mismatch")

				g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())

				return true
			},
		},
		{
			name: "FIPS enabled rejects non-compliant root key",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: &rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "Red Hat",
							PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
								LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
									Name: "tsa-invalid-secret",
								},
								Key: "rootPrivateKey",
							},
							PasswordRef: &rhtasv1alpha1.SecretKeySelector{
								LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
									Name: "tsa-invalid-secret",
								},
								Key: "rootPrivateKeyPassword",
							},
						},
					},
				}
				g := NewWithT(t)
				_, invalidPriv, _, err := fipsTest.GenerateECCertificatePEM(false, "", elliptic.P224())
				g.Expect(err).NotTo(HaveOccurred())
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tsa-invalid-secret",
						Namespace: instance.Namespace,
					},
					Data: map[string][]byte{
						"rootPrivateKey": invalidPriv,
					},
				}
				cryptoutil.FIPSEnabled = true
				t.Cleanup(func() {
					cryptoutil.FIPSEnabled = false
				})

				cli, act := common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), secret)
				return cli, act
			},
			testCase: func(g Gomega, _ action.Action[*rhtasv1alpha1.TimestampAuthority], _ client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				cond := meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition)
				g.Expect(cond).NotTo(BeNil(), "TSASignerCondition should be present")
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse), "TSASignerCondition status should be False")
				g.Expect(cond.Reason).To(Equal(constants.Failure), "TSASignerCondition reason should be Failure")

				return true
			},
		},
		{
			name: "FIPS enabled rejects non-compliant certificate chain ref",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						CertificateChainRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "tsa-invalid-cert",
							},
							Key: "certificateChain",
						},
					},
					Kms: &rhtasv1alpha1.KMS{
						KeyResource: "gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/k",
					},
				}

				g := NewWithT(t)
				_, _, invalidCert, err := fipsTest.GenerateECCertificatePEM(true, "pass", elliptic.P224())
				g.Expect(err).NotTo(HaveOccurred())

				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "tsa-invalid-cert",
						Namespace: instance.Namespace,
					},
					Data: map[string][]byte{
						"certificateChain": invalidCert,
					},
				}

				cryptoutil.FIPSEnabled = true
				t.Cleanup(func() {
					cryptoutil.FIPSEnabled = false
				})

				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), secret)
			},
			testCase: func(g Gomega, _ action.Action[*rhtasv1alpha1.TimestampAuthority], _ client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				cond := meta.FindStatusCondition(instance.Status.Conditions, TSASignerCondition)
				g.Expect(cond).NotTo(BeNil(), "TSASignerCondition should be present")
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse), "TSASignerCondition status should be False")
				g.Expect(cond.Reason).To(Equal(constants.Failure), "TSASignerCondition reason should be Failure")

				return true
			},
		},
		{
			name: "user-spec keys and certs",
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
				secret := tsa.CreateSecrets(instance.Namespace, "tsa-test-secret")
				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), secret)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], client client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				secret, err := kubernetes.FindSecret(context.TODO(), client, instance.Namespace, TSACertCALabel)
				g.Expect(err).NotTo(HaveOccurred(), "Unable to find secret")

				g.Expect(instance.Status.Signer).NotTo(BeNil(), "Status Signer should not be nil")

				g.Expect(instance.Status.Signer.CertificateChain.CertificateChainRef.Name).To(Equal("tsa-test-secret"))

				g.Expect(secret.Annotations).To(Equal(generateSecretAnnotations(instance.Spec.Signer)), "Secret annotation mismatch")

				g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())

				return true
			},
		},
		{
			name: "update cert secret resource on key change",
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
				instance.Status.Signer = &rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						CertificateChainRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "old",
							},
							Key: "certificateChain",
						},
					},
					File: &rhtasv1alpha1.File{
						PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "old",
							},
							Key: "leafPrivateKey",
						},
						PasswordRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "old",
							},
							Key: "leafPrivateKeyPassword",
						},
					},
				}

				secret := tsa.CreateSecrets(instance.Namespace, "tsa-test-secret")
				old := tsa.CreateSecrets(instance.Namespace, "old")
				old.Annotations = generateSecretAnnotations(*instance.Status.Signer)
				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), secret, old)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], cli client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				secret, err := kubernetes.FindSecret(context.TODO(), cli, instance.Namespace, TSACertCALabel)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to find secret")

				g.Expect(instance.Status.Signer).NotTo(BeNil(), "Signer should not be nil")

				g.Expect(instance.Status.Signer.CertificateChain.CertificateChainRef.Name).To(Equal("tsa-test-secret"), "Secret name mismatch for CertificateChainRef")
				g.Expect(instance.Status.Signer.File.PrivateKeyRef.Name).To(Equal("tsa-test-secret"), "Private key ref name mismatch for PrivateKeyRef")

				g.Expect(secret.Annotations).To(Equal(generateSecretAnnotations(instance.Spec.Signer)), "Secret annotation mismatch")

				old := &v1.Secret{}
				g.Expect(cli.Get(context.TODO(), client.ObjectKey{Name: "old", Namespace: "default"}, old)).To(Succeed())
				g.Expect(old.Labels).ToNot(HaveKey(TSACertCALabel))

				g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())

				return true
			},
		},
		{
			name: "update cert secret resource on cert field change",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: &rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "Red Hat",
						},
						IntermediateCA: []*rhtasv1alpha1.TsaCertificateAuthority{
							{
								OrganizationName: "Red Hat",
							},
						},
						LeafCA: &rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "Red Hat",
						},
					},
				}
				instance.Status.Signer = &rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						CertificateChainRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "old",
							},
							Key: "certificateChain",
						},
					},
					File: &rhtasv1alpha1.File{
						PrivateKeyRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "old",
							},
							Key: "leafPrivateKey",
						},
						PasswordRef: &rhtasv1alpha1.SecretKeySelector{
							LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
								Name: "old",
							},
							Key: "leafPrivateKeyPassword",
						},
					},
				}

				old := tsa.CreateSecrets(instance.Namespace, "old")
				old.Annotations = generateSecretAnnotations(*instance.Status.Signer)
				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), old)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], cli client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				secret, err := kubernetes.FindSecret(context.TODO(), cli, instance.Namespace, TSACertCALabel)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to find secret")

				g.Expect(instance.Status.Signer).NotTo(BeNil(), "Signer should not be nil")
				g.Expect(instance.Status.Signer.CertificateChain.CertificateChainRef.Name).NotTo(Equal("old"), "Signer should be updated")

				g.Expect(instance.Status.Signer.CertificateChain.RootCA.OrganizationName).To(Equal("Red Hat"), "Secret name mismatch for CertificateChainRef")

				g.Expect(secret.Annotations).To(Equal(generateSecretAnnotations(instance.Spec.Signer)), "Secret annotation mismatch")

				old := &v1.Secret{}
				g.Expect(cli.Get(context.TODO(), client.ObjectKey{Name: "old", Namespace: "default"}, old)).To(Succeed())
				g.Expect(old.Labels).ToNot(HaveKey(TSACertCALabel))

				g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())

				return true
			},
		},
		{
			name: "find existing secret",
			setup: func(instance *rhtasv1alpha1.TimestampAuthority) (client.WithWatch, action.Action[*rhtasv1alpha1.TimestampAuthority]) {
				instance.Status.Conditions[0].Reason = constants.Pending
				instance.Spec.Signer = rhtasv1alpha1.TimestampAuthoritySigner{
					CertificateChain: rhtasv1alpha1.CertificateChain{
						RootCA: &rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "Red Hat",
						},
						IntermediateCA: []*rhtasv1alpha1.TsaCertificateAuthority{
							{
								OrganizationName: "Red Hat",
							},
						},
						LeafCA: &rhtasv1alpha1.TsaCertificateAuthority{
							OrganizationName: "Red Hat",
						},
					},
				}

				secret := tsa.CreateSecrets(instance.Namespace, "secret")
				secret.Annotations = generateSecretAnnotations(instance.Spec.Signer)
				secret.Labels = map[string]string{TSACertCALabel: "fake"}
				return common.TsaTestSetup(instance, t, nil, NewGenerateSignerAction(), secret)
			},
			testCase: func(g Gomega, a action.Action[*rhtasv1alpha1.TimestampAuthority], cli client.WithWatch, instance *rhtasv1alpha1.TimestampAuthority) bool {
				secret, err := kubernetes.FindSecret(context.TODO(), cli, instance.Namespace, TSACertCALabel)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to find secret")

				g.Expect(instance.Status.Signer).NotTo(BeNil(), "Signer should not be nil")
				g.Expect(instance.Status.Signer.CertificateChain.CertificateChainRef.Name).To(Equal("secret"), "Signer should be updated")

				g.Expect(instance.Status.Signer.CertificateChain.RootCA.OrganizationName).To(Equal("Red Hat"), "Secret name mismatch for CertificateChainRef")

				g.Expect(secret.Labels).To(HaveKey(TSACertCALabel), "Secret labels mismatch")

				g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, TSASignerCondition)).To(BeTrue())

				return true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			instance := common.GenerateTSAInstance()
			cli, act := tt.setup(instance)
			g.Expect(cli).NotTo(BeNil(), "Client should not be nil")
			g.Expect(act).NotTo(BeNil(), "Action should not be nil")
			g.Expect(cli.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(tt.testCase(g, act, cli, instance)).To(BeTrue())
		})
	}
}

func generateSecretAnnotations(signer rhtasv1alpha1.TimestampAuthoritySigner) map[string]string {
	annotations := make(map[string]string)
	bytes, _ := json.Marshal(signer)
	annotations[labels.LabelNamespace+"/signerConfiguration"] = string(bytes)
	return annotations
}
