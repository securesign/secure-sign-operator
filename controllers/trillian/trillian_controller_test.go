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

package trillian

import (
	"context"
	"time"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/constants"
	actions "github.com/securesign/operator/controllers/trillian/actions"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Trillian controller", func() {
	Context("Trillian controller test", func() {

		const (
			Name      = "test"
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
		trillian := &v1alpha1.Trillian{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Trillian")
			found := &v1alpha1.Trillian{}
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

		It("should successfully reconcile a custom resource for Trillian", func() {
			By("creating the custom resource for the Kind Trillian")
			err := k8sClient.Get(ctx, typeNamespaceName, trillian)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				trillian := &v1alpha1.Trillian{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: v1alpha1.TrillianSpec{
						Db: v1alpha1.TrillianDB{
							Create: true,
						},
					},
				}
				err = k8sClient.Create(ctx, trillian)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &v1alpha1.Trillian{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func() bool {
				found := &v1alpha1.Trillian{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.Ready, metav1.ConditionFalse)
			}, time.Minute, time.Second).Should(BeTrue())
			found := &v1alpha1.Trillian{}

			By("Database secret created")
			Eventually(func() *corev1.LocalObjectReference {
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Spec.Db.DatabaseSecretRef
			}, time.Minute, time.Second).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Spec.Db.DatabaseSecretRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Database PVC created")
			Eventually(func() string {
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Spec.Db.PvcName
			}, time.Minute, time.Second).Should(Not(BeNil()))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Spec.Db.PvcName, Namespace: Namespace}, &corev1.PersistentVolumeClaim{})).Should(Succeed())

			By("Database SVC created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "trillian-mysql", Namespace: Namespace}, &corev1.Service{})
			}, time.Minute, time.Second).Should(Succeed())

			By("Database Deployment created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.DbDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}, time.Minute, time.Second).Should(Succeed())

			By("LogServer Deployment created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.LogserverDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}, time.Minute, time.Second).Should(Succeed())

			By("LogServerSvc Deployment created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.LogserverDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}, time.Minute, time.Second).Should(Succeed())

			By("LogSigner Deployment created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.LogsignerDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}, time.Minute, time.Second).Should(Succeed())

			By("Waiting until Trillian instance is Initialization")
			Eventually(func() string {
				found := &v1alpha1.Trillian{}
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

			By("Waiting until Trillian instance is Ready")
			Eventually(func() bool {
				found := &v1alpha1.Trillian{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}, time.Minute, time.Second).Should(BeTrue())

			By("Checking if controller will return deployment to desired state")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.LogserverDeploymentName, Namespace: Namespace}, deployment)
			}, time.Minute, time.Second).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func() int32 {
				deployment = &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.LogserverDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}, time.Minute, time.Second).Should(Equal(int32(1)))
		})
	})
})
