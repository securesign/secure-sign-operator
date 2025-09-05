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

package ctlog

import (
	"maps"
	"time"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes"

	"github.com/securesign/operator/internal/controller/ctlog/utils"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	fulcio "github.com/securesign/operator/internal/controller/fulcio/actions"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CTlog update test", func() {
	Context("CTlog update test", func() {

		const (
			Name      = "test"
			Namespace = "update"
		)

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		instance := &v1alpha1.CTlog{}

		BeforeEach(func(ctx SpecContext) {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func(ctx SpecContext) {
			By("removing the custom resource for the Kind CTlog")
			found := &v1alpha1.CTlog{}
			err := suite.Client().Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return suite.Client().Delete(ctx, found)
			}, 3*time.Minute, time.Second).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for CTlog", func(ctx SpecContext) {
			By("creating the custom resource for the Kind CTlog")
			err := suite.Client().Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				ptr := int64(1)
				instance := &v1alpha1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},

					Spec: v1alpha1.CTlogSpec{
						TreeID: &ptr,
					},
				}
				err = suite.Client().Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))

			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &v1alpha1.CTlog{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Creating trillian service")
			Expect(suite.Client().Create(ctx, kubernetes.CreateService(Namespace, trillian.LogserverDeploymentName, trillian.ServerPortName, trillian.ServerPort, trillian.ServerPort, labels.ForComponent(trillian.LogServerComponentName, instance.Name)))).To(Succeed())

			By("Creating fulcio root cert")
			fulcioCa := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: Namespace,
					Labels:    map[string]string{fulcio.FulcioCALabel: "cert"},
				},
				Data: map[string][]byte{"cert": []byte("fakeCert")},
			}
			Expect(suite.Client().Create(ctx, fulcioCa)).To(Succeed())

			deployment := &appsv1.Deployment{}
			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Waiting until CTlog instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}).Should(BeTrue())

			By("Fulcio CA has changed")
			// invalidate
			maps.DeleteFunc(fulcioCa.Labels, func(key string, val string) bool {
				return key == fulcio.FulcioCALabel
			})
			Expect(suite.Client().Update(ctx, fulcioCa)).To(Succeed())

			fulcioCa = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test2",
					Namespace: Namespace,
					Labels:    map[string]string{fulcio.FulcioCALabel: "cert"},
				},
				Data: map[string][]byte{"cert": []byte("fakeCert2")},
			}
			Expect(suite.Client().Create(ctx, fulcioCa)).To(Succeed())

			By("CA has changed in status field")
			Eventually(func(g Gomega) {
				found := &v1alpha1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())

				// both certs are present
				g.Expect(found.Status.RootCertificates).
					Should(And(
						ContainElement(WithTransform(func(ks v1alpha1.SecretKeySelector) string { return ks.Name }, Equal("test"))),
						ContainElement(WithTransform(func(ks v1alpha1.SecretKeySelector) string { return ks.Name }, Equal("test2"))),
					),
					)
			}).Should(Succeed())

			By("CTL deployment is updated")
			Eventually(func() bool {
				updated := &appsv1.Deployment{}
				Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).Should(BeFalse())

			By("Move to Ready phase")
			deployment = &appsv1.Deployment{}
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Private key has changed")
			key, err := utils.CreatePrivateKey(nil)
			Expect(err).To(Not(HaveOccurred()))
			Expect(suite.Client().Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "key-secret",
					Namespace: Namespace,
					Labels:    labels.For(actions.ComponentName, Name, instance.Name),
				},
				Data: map[string][]byte{"private": key.PrivateKey, "password": key.PrivateKeyPass},
			})).To(Succeed())

			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())
			found := &v1alpha1.CTlog{}
			Eventually(func(g Gomega) error {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.PrivateKeyRef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "key-secret",
					},
					Key: "private",
				}
				found.Spec.PrivateKeyPasswordRef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "key-secret",
					},
					Key: "password",
				}
				return suite.Client().Update(ctx, found)
			}).Should(Succeed())

			By("CTLog status field changed")
			Eventually(func(g Gomega) string {
				found := &v1alpha1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PrivateKeyRef.Name
			}).Should(Equal("key-secret"))

			By("CTL deployment is updated")
			Eventually(func(g Gomega) bool {
				updated := &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).Should(BeFalse())
		})
	})
})
