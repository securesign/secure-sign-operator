package tsa

/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"context"
	"time"

	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/tsa/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Timestamp Authority hot update", func() {
	Context("Timestamp Authority hot update test", func() {

		const (
			Name      = "test-tsa"
			Namespace = "update"
		)

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		timestampAuthority := &rhtasv1alpha1.TimestampAuthority{}
		found := &rhtasv1alpha1.TimestampAuthority{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Timestamp Authority")
			err := k8sClient.Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return k8sClient.Delete(context.TODO(), found)
			}, 2*time.Minute, time.Second).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = k8sClient.Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for the Timestamp Authority", func() {
			By("creating the custom resource for the Timestamp Authority")
			err := k8sClient.Get(ctx, typeNamespaceName, timestampAuthority)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				tsa := &rhtasv1alpha1.TimestampAuthority{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: rhtasv1alpha1.TimestampAuthoritySpec{
						ExternalAccess: rhtasv1alpha1.ExternalAccess{
							Host:    "tsa.localhost",
							Enabled: true,
						},
						Monitoring: rhtasv1alpha1.MonitoringConfig{Enabled: false},
						Signer: rhtasv1alpha1.TimestampAuthoritySigner{
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
						},
						NTPMonitoring: rhtasv1alpha1.NTPMonitoring{
							Enabled: true,
							Config: &rhtasv1alpha1.NtpMonitoringConfig{
								RequestAttempts: 3,
								RequestTimeout:  5,
								NumServers:      4,
								ServerThreshold: 3,
								MaxTimeDelta:    6,
								Period:          60,
								Servers:         []string{"time.apple.com", "time.google.com"},
							},
						},
					},
				}
				err = k8sClient.Create(ctx, tsa)
				Expect(err).To(Not(HaveOccurred()))
			}
			deployment := &appsv1.Deployment{}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, k8sClient, deployment)).To(Succeed())

			By("Waiting until Timestamp Authority is Ready")
			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}).Should(BeTrue())

			By("Cert and Key rotation")
			Eventually(func(g Gomega) error {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.Signer.CertificateChain = rhtasv1alpha1.CertificateChain{
					CertificateChainRef: &rhtasv1alpha1.SecretKeySelector{
						LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
							Name: "tsa-test-secret",
						},
						Key: "certificateChain",
					},
				}

				found.Spec.Signer.File = &rhtasv1alpha1.File{
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
				}
				return k8sClient.Update(ctx, found)
			}).Should(Succeed())

			By("Pending phase until new keys and certs are resolved")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Pending))

			By("Creating new certificate chain and signer keys")
			secret := tsa.CreateSecrets(Namespace, "tsa-test-secret")
			Expect(k8sClient.Create(context.TODO(), secret)).NotTo(HaveOccurred())

			By("Status field changed for cert chain")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.CertificateChain.CertificateChainRef.Name
			}).Should(Equal("tsa-test-secret"))

			By("Status field changed for signer key")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.File.PasswordRef.Name
			}).Should(Equal("tsa-test-secret"))

			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, actions.TSASignerCondition)
			}).Should(BeTrue())

			By("Timestamp Authority deployment is updated")
			Eventually(func(g Gomega) bool {
				updated := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).Should(BeFalse())

			By("Move to Ready phase")
			deployment = &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())
			Expect(k8sTest.SetDeploymentToReady(ctx, k8sClient, deployment)).To(Succeed())

			time.Sleep(10 * time.Second)

			By("NTP Monitoring update")
			By("NTP monitoring config should be created")
			Eventually(func(g Gomega) *rhtasv1alpha1.LocalObjectReference {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.NTPMonitoring.Config.NtpConfigRef
			}).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Status.NTPMonitoring.Config.NtpConfigRef.Name, Namespace: Namespace}, &corev1.ConfigMap{})).Should(Succeed())

			By("Update NTP Config")
			Eventually(func(g Gomega) error {
				err := k8sClient.Get(ctx, typeNamespaceName, found)
				if err != nil {
					return err
				}
				found.Spec.NTPMonitoring.Config.NumServers = 2
				return k8sClient.Update(ctx, found)
			}).WithTimeout(1 * time.Second).Should(Succeed())

			By("NTP monitoring should be resolved")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Message
			}).Should(Equal("Waiting for deployment to be ready"))

			By("Timestamp Authority deployment is updated")
			Eventually(func(g Gomega) bool {
				updated := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).Should(BeFalse())

			By("Move to Ready phase")
			deployment = &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())
			Expect(k8sTest.SetDeploymentToReady(ctx, k8sClient, deployment)).To(Succeed())
		})
	})
})
