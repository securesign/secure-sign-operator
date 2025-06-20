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

package tuf

import (
	"context"
	"maps"
	"time"

	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apilabels "k8s.io/apimachinery/pkg/labels"

	"github.com/securesign/operator/api/v1alpha1"
	actions2 "github.com/securesign/operator/internal/controller/ctlog/actions"
	batchv1 "k8s.io/api/batch/v1"
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

var _ = Describe("TUF controller", func() {
	Context("TUF controller test", func() {

		const (
			TufName      = "test-tuf"
			TufNamespace = "controller"
		)

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: TufNamespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: TufName, Namespace: TufNamespace}
		tuf := &v1alpha1.Tuf{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Tuf")
			found := &v1alpha1.Tuf{}
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

		It("should successfully reconcile a custom resource for Tuf", func() {
			By("creating the custom resource for the Kind Tuf")
			err := suite.Client().Get(ctx, typeNamespaceName, tuf)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				tuf := &v1alpha1.Tuf{
					ObjectMeta: metav1.ObjectMeta{
						Name:      TufName,
						Namespace: TufNamespace,
					},
					Spec: v1alpha1.TufSpec{
						ExternalAccess: v1alpha1.ExternalAccess{
							Host:    "tuf.localhost",
							Enabled: true,
						},
						Port: 8181,
						Keys: []v1alpha1.TufKey{
							{
								Name: "fulcio_v1.crt.pem",
								SecretRef: &v1alpha1.SecretKeySelector{
									LocalObjectReference: v1alpha1.LocalObjectReference{
										Name: "fulcio-pub-key",
									},
									Key: "cert",
								},
							},
							{
								Name: "ctfe.pub",
							},
							{
								Name: "rekor.pub",
								SecretRef: &v1alpha1.SecretKeySelector{
									LocalObjectReference: v1alpha1.LocalObjectReference{
										Name: "rekor-pub-key",
									},
									Key: "public",
								},
							},
						},
					},
				}
				err = suite.Client().Create(ctx, tuf)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &v1alpha1.Tuf{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.Ready, metav1.ConditionFalse)
			}).Should(BeTrue())

			By("Pending phase until ctlog public key is resolved")
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Pending))

			By("Creating ctlog secret with public key")
			secretLabels := map[string]string{
				labels.LabelNamespace + "/ctfe.pub": "public",
			}
			maps.Copy(secretLabels, labels.For(actions2.ComponentName, actions2.ComponentName, actions2.ComponentName))
			_ = suite.Client().Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog-test",
					Namespace: typeNamespaceName.Namespace,
					Labels:    secretLabels,
				},
				Data: map[string][]byte{
					"public": []byte("secret"),
				},
			})

			By("Waiting until Tuf init job is created")
			found := &v1alpha1.Tuf{}
			Eventually(func(g Gomega) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PvcName
			}).ShouldNot(BeEmpty())
			initJobList := &batchv1.JobList{}
			Eventually(func() []batchv1.Job {
				jobLabels := labels.ForResource(tufConstants.ComponentName, tufConstants.InitJobName, TufName, found.Status.PvcName)
				selector := apilabels.SelectorFromSet(jobLabels)
				Expect(kubernetes.FindByLabelSelector(ctx, suite.Client(), initJobList, TufNamespace, selector.String())).To(Succeed())
				return initJobList.Items
			}).Should(HaveLen(1))

			By("Move to Job to completed")
			// Workaround to succeed condition for Ready phase
			initJob := &initJobList.Items[0]
			initJob.Status.Conditions = []batchv1.JobCondition{
				{Status: corev1.ConditionTrue, Type: batchv1.JobComplete, Reason: constants.Ready}}
			Expect(suite.Client().Status().Update(ctx, initJob)).Should(Succeed())

			By("Repository condition gets ready")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, tufConstants.RepositoryCondition)
			}).Should(BeTrue())

			By("Waiting until Tuf instance is Initialization")
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Initialize))

			deployment := &appsv1.Deployment{}
			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, deployment)
			}).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Waiting until Tuf instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}).Should(BeTrue())

			By("Checking if Service was successfully created in the reconciliation")
			service := &corev1.Service{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, service)
			}).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).Should(Equal(int32(8181)))

			By("Checking if Ingress was successfully created in the reconciliation")
			ingress := &v1.Ingress{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, ingress)
			}).Should(Succeed())
			Expect(ingress.Spec.Rules[0].Host).Should(Equal("tuf.localhost"))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).Should(Equal(service.Name))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).Should(Equal(tufConstants.PortName))

			By("Checking the latest Status Condition added to the Tuf instance")
			Eventually(func(g Gomega) error {
				found := &v1alpha1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				rekorCondition := meta.FindStatusCondition(found.Status.Conditions, "rekor.pub")
				g.Expect(rekorCondition).Should(Not(BeNil()))
				g.Expect(rekorCondition.Status).Should(Equal(metav1.ConditionTrue))
				g.Expect(rekorCondition.Reason).Should(Equal("Ready"))
				ctlogCondition := meta.FindStatusCondition(found.Status.Conditions, "ctfe.pub")
				g.Expect(ctlogCondition).Should(Not(BeNil()))
				g.Expect(ctlogCondition.Status).Should(Equal(metav1.ConditionTrue))
				g.Expect(ctlogCondition.Reason).Should(Equal("Ready"))
				return nil
			}).Should(Succeed())

			By("Checking if controller will return deployment to desired state")
			deployment = &appsv1.Deployment{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, deployment)
			}).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(suite.Client().Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func(g Gomega) int32 {
				deployment = &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}).Should(Equal(int32(1)))
		})
	})
})
