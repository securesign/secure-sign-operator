/*
Copyright 2026.

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

package console

import (
	"context"
	"time"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	"github.com/securesign/operator/internal/utils"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/console/actions"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Console controller", func() {
	Context("Console controller test", func() {

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
		console := &v1alpha1.Console{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Console")
			found := &v1alpha1.Console{}
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

		It("should successfully reconcile a custom resource for Console", func() {
			By("creating the custom resource for the Kind Console")
			err := suite.Client().Get(ctx, typeNamespaceName, console)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				console := &v1alpha1.Console{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: v1alpha1.ConsoleSpec{
						Enabled: true,
						Db: v1alpha1.ConsoleDB{
							Create: utils.Pointer(true),
							Pvc: v1alpha1.Pvc{
								Size: utils.Pointer(resource.MustParse("1Gi")),
							},
						},
						Api: v1alpha1.ConsoleApi{
							Tuf: v1alpha1.TufService{
								Address: "http://tuf.default.svc",
							},
						},
						UI: v1alpha1.ConsoleUI{
							ExternalAccess: v1alpha1.ExternalAccess{
								Enabled: false,
							},
						},
					},
				}
				err = suite.Client().Create(ctx, console)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &v1alpha1.Console{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Console{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.ReadyCondition, metav1.ConditionFalse)
			}).Should(BeTrue())
			found := &v1alpha1.Console{}

			By("Database secret created")
			Eventually(func(g Gomega) *v1alpha1.LocalObjectReference {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Db.DatabaseSecretRef
			}).Should(Not(BeNil()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Db.DatabaseSecretRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Database PVC created")
			Eventually(func(g Gomega) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Db.Pvc.Name
			}).Should(Not(BeEmpty()))

			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Db.Pvc.Name, Namespace: Namespace}, &corev1.PersistentVolumeClaim{})).Should(Succeed())

			By("Database Service created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DbDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("Database Deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DbDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("API Deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("API Service created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("UI Deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.UIDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("UI Service created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.UIDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("Waiting until Console instance is Initialization")
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Console{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition).Reason
			}).Should(Equal(state.Initialize.String()))

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			deployments := &appsv1.DeploymentList{}
			Expect(suite.Client().List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			for _, d := range deployments.Items {
				Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), &d)).To(Succeed())
			}

			By("Waiting until Console instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Console{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).Should(BeTrue())

			By("Checking if controller will return deployment to desired state")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(suite.Client().Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func(g Gomega) int32 {
				deployment = &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}).Should(Equal(int32(1)))
		})
	})
})
