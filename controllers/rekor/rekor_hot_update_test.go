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

package rekor

import (
	"context"
	"time"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	trillian "github.com/securesign/operator/controllers/trillian/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rekor hot update test", func() {
	Context("Rekor update test", func() {

		const (
			Name      = "test"
			Namespace = "update"
		)

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		instance := &v1alpha1.Rekor{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Rekor")
			found := &v1alpha1.Rekor{}
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

		It("should successfully reconcile a custom resource for Rekor", func() {
			By("creating the custom resource for the Kind Rekor")
			err := k8sClient.Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				ptr := int64(123)
				instance := &v1alpha1.Rekor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: v1alpha1.RekorSpec{
						TreeID: &ptr,
						ExternalAccess: v1alpha1.ExternalAccess{
							Enabled: false,
						},
						RekorSearchUI: v1alpha1.RekorSearchUI{
							Enabled: false,
						},
						BackFillRedis: v1alpha1.BackFillRedis{
							Enabled: false,
						},
					},
				}
				err = k8sClient.Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &v1alpha1.Rekor{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Move to CreatingPhase by creating trillian service")
			Expect(k8sClient.Create(ctx, kubernetes.CreateService(Namespace, trillian.LogserverDeploymentName, 8091, constants.LabelsForComponent(trillian.LogServerComponentName, instance.Name)))).To(Succeed())

			By("Waiting until Rekor instance is Initialization")
			Eventually(func() string {
				found := &v1alpha1.Rekor{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}, time.Minute, time.Second).Should(Equal(constants.Initialize))

			deployments := &appsv1.DeploymentList{}
			Expect(k8sClient.List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			By("Move to Ready phase")
			for _, d := range deployments.Items {
				d.Status.Replicas = *d.Spec.Replicas
				d.Status.ReadyReplicas = *d.Spec.Replicas
				Expect(k8sClient.Status().Update(ctx, &d)).Should(Succeed())
			}
			// Workaround to succeed condition for Ready phase

			By("Waiting until Rekor instance is Ready")
			found := &v1alpha1.Rekor{}
			Eventually(func() bool {

				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}, time.Minute, time.Second).Should(BeTrue())

			By("Save the Deployment configuration")
			deployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())

			By("Patch the signer key")
			Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
			found.Spec.Signer.KeyRef = &v1alpha1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "key-secret",
				},
				Key: "private",
			}
			Expect(k8sClient.Update(ctx, found)).To(Succeed())
			By("Move to CreatingPhase by creating trillian service")
			Expect(k8sClient.Create(ctx, kubernetes.CreateSecret("key-secret", Namespace, map[string][]byte{"private": []byte("fake")}, constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)))).To(Succeed())

			By("Secret key is resolved")
			Eventually(func() *v1alpha1.SecretKeySelector {
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.KeyRef
			}, time.Minute, time.Second).Should(Not(BeNil()))
			Eventually(func() string {
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.KeyRef.Name
			}, time.Minute, time.Second).Should(Equal("key-secret"))

			By("Rekor deployment is updated")
			Eventually(func() bool {
				updated := &appsv1.Deployment{}
				k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, updated)
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}, time.Minute, time.Second).Should(BeFalse())
		})
	})
})
