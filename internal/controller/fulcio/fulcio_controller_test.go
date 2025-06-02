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

package fulcio

import (
	"context"
	"time"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Fulcio controller", func() {
	Context("Fulcio controller test", func() {

		const (
			Name      = "test-fulcio"
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
		instance := &v1alpha1.Fulcio{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Fulcio")
			found := &v1alpha1.Fulcio{}
			err := suite.Client().Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return suite.Client().Delete(context.TODO(), found)
			}, 2*time.Minute, time.Second).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for Fulcio", func() {
			By("creating the custom resource for the Kind Fulcio")
			err := suite.Client().Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				instance := &v1alpha1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: v1alpha1.FulcioSpec{
						ExternalAccess: v1alpha1.ExternalAccess{
							Host:    "fulcio.localhost",
							Enabled: true,
						},
						Config: v1alpha1.FulcioConfig{
							OIDCIssuers: []v1alpha1.OIDCIssuer{
								{
									IssuerURL: "test",
									Issuer:    "test",
									ClientID:  "test",
									Type:      "email",
								},
							},
						},
						Certificate: v1alpha1.FulcioCert{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.com",
							CommonName:        "local",
							PrivateKeyPasswordRef: &v1alpha1.SecretKeySelector{
								LocalObjectReference: v1alpha1.LocalObjectReference{
									Name: "password-secret",
								},
								Key: "password",
							},
						},
						Monitoring: v1alpha1.MonitoringConfig{Enabled: false},
						TrustedCA: &v1alpha1.LocalObjectReference{
							Name: "trusted-ca-bundle",
						},
					},
				}
				err = suite.Client().Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &v1alpha1.Fulcio{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Fulcio{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.Ready, metav1.ConditionFalse)
			}).Should(BeTrue())

			By("Pending phase until password key is resolved")
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Fulcio{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Pending))

			By("Creating password secret with cert password")
			Expect(suite.Client().Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "password-secret",
					Namespace: typeNamespaceName.Namespace,
					Labels:    labels.ForComponent(actions.ComponentName, instance.Name),
				},
				Data: map[string][]byte{
					"password": []byte("secret"),
				},
			})).To(Succeed())

			By("Secrets are resolved")
			var certSecretPartialObject *metav1.PartialObjectMetadata
			var certSecret *corev1.Secret
			Eventually(func(g Gomega) *corev1.Secret {
				certSecretPartialObject, err = kubernetes.FindSecret(ctx, suite.Client(), Namespace, actions.FulcioCALabel)
				g.Expect(err).To(Not(HaveOccurred()))
				certSecret, err = kubernetes.GetSecret(suite.Client(), certSecretPartialObject.Namespace, certSecretPartialObject.Name)
				g.Expect(err).To(Not(HaveOccurred()))
				return certSecret
			}).Should(Not(BeNil()))

			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Fulcio{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, actions.CertCondition)
			}).Should(BeTrue())
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Fulcio{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Certificate.CARef.Name
			}).Should(Equal(certSecret.Name))
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Fulcio{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Certificate.PrivateKeyRef.Name
			}).Should(Equal(certSecret.Name))
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Fulcio{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Certificate.PrivateKeyPasswordRef.Name
			}).Should(Equal("password-secret"))

			Expect(certSecret.Data).To(And(HaveKey("private"), HaveKey("cert")))

			deployment := &appsv1.Deployment{}
			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Waiting until Fulcio instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Fulcio{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}).Should(BeTrue())

			By("Checking if Service was successfully created in the reconciliation")
			service := &corev1.Service{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, service)
			}).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).Should(Equal(int32(80)))
			Expect(service.Spec.Ports[1].Port).Should(Equal(int32(5554)))

			By("Checking if Ingress was successfully created in the reconciliation")
			ingress := &v1.Ingress{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, ingress)
			}).Should(Succeed())
			Expect(ingress.Spec.Rules[0].Host).Should(Equal("fulcio.localhost"))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).Should(Equal(service.Name))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).Should(Equal(actions.ServerPortName))

			By("Checking if controller will return deployment to desired state")
			deployment = &appsv1.Deployment{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(suite.Client().Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func(g Gomega) int32 {
				deployment = &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}).Should(Equal(int32(1)))
		})
	})
})
