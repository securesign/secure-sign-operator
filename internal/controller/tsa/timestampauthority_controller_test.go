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

package tsa

import (
	"context"
	"time"

	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/tsa/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
)

var _ = Describe("TimestampAuthority Controller", func() {
	Context("When reconciling a resource", func() {

		const (
			Name      = "test-tsa"
			Namespace = "default"
		)

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      Name,
				Namespace: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		timestampAuthority := &rhtasv1alpha1.TimestampAuthority{}
		found := &rhtasv1alpha1.TimestampAuthority{}
		deployment := &appsv1.Deployment{}
		service := &corev1.Service{}
		ingress := &v1.Ingress{}

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

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.Ready, metav1.ConditionFalse)
			}).Should(BeTrue())

			By("Tsa signer should be resolved")
			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, actions.TSASignerCondition, metav1.ConditionTrue)
			}).Should(BeTrue())

			By("Certificate chain secret should be created")
			Eventually(func(g Gomega) *rhtasv1alpha1.SecretKeySelector {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.CertificateChain.CertificateChainRef
			}).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Status.Signer.CertificateChain.CertificateChainRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("File Signer secret should be created")
			Eventually(func(g Gomega) *rhtasv1alpha1.SecretKeySelector {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.File.PrivateKeyRef
			}).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Status.Signer.File.PrivateKeyRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Should be in a creating phase")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Creating))

			By("NTP monitoring should be resolved")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Message
			}).Should(Equal("Waiting for deployment to be ready"))

			By("NTP monitoring config should be created")
			Eventually(func(g Gomega) *rhtasv1alpha1.LocalObjectReference {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.NTPMonitoring.Config.NtpConfigRef
			}).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Status.NTPMonitoring.Config.NtpConfigRef.Name, Namespace: Namespace}, &corev1.ConfigMap{})).Should(Succeed())

			By("Timestamp Authority service is created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, service)
			}).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).Should(Equal(int32(3000)))

			By("Checking if Ingress was successfully created in the reconciliation")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, ingress)
			}).Should(Succeed())
			Expect(ingress.Spec.Rules[0].Host).Should(Equal("tsa.localhost"))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).Should(Equal(service.Name))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).Should(Equal(actions.ServerPortName))

			By("Timestamp Authority deployment is created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, k8sClient, deployment)).To(Succeed())

			By("Waiting until Timestamp Authority instance is Ready")
			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}).Should(BeTrue())

		})
	})
})
