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
	"context"
	"os"
	"time"

	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	fulcio "github.com/securesign/operator/internal/controller/fulcio/actions"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	appsv1 "k8s.io/api/apps/v1"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CTlog ErrorHandler", func() {
	Context("CTlog ErrorHandler test", func() {

		const (
			Name      = "test"
			Namespace = "errorhandler"
		)

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		instance := &v1alpha1.CTlog{}

		BeforeEach(func() {
			// workaround - disable "host" mode in CreateTrillianTree function
			Expect(os.Setenv("CONTAINER_MODE", "true")).To(Not(HaveOccurred()))

			By("Creating the Namespace to perform the tests")
			err := k8sClient.Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind CTlog")
			found := &v1alpha1.CTlog{}
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

		It("should successfully reconcile a custom resource for CTlog", func() {
			By("creating the custom resource for the Kind CTlog")
			err := k8sClient.Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				instance := &v1alpha1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: v1alpha1.CTlogSpec{},
				}
				err = k8sClient.Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))
			}

			Expect(k8sClient.Create(ctx, kubernetes.CreateSecret("test", Namespace,
				map[string][]byte{"cert": []byte("fakeCert")},
				map[string]string{fulcio.FulcioCALabel: "cert"},
			))).To(Succeed())

			Expect(k8sClient.Create(ctx, kubernetes.CreateService(Namespace, trillian.LogserverDeploymentName, trillian.ServerPortName, trillian.ServerPort, trillian.ServerPort, constants.LabelsForComponent(trillian.LogServerComponentName, instance.Name)))).To(Succeed())

			found := &v1alpha1.CTlog{}

			By("Deployment should fail - trillian server is not running")
			Eventually(func(g Gomega) string {

				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				condition := meta.FindStatusCondition(found.Status.Conditions, constants.Ready)
				if condition == nil {
					return ""
				}

				return condition.Reason
			}).Should(Equal(constants.Error))

			key := found.Status.PrivateKeyRef.Name
			Expect(key).To(Not(BeEmpty()))

			By("Periodically trying to restart deployment")
			Eventually(func(g Gomega) v1alpha1.CTlogStatus {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status
			}).Should(And(
				WithTransform(
					func(status v1alpha1.CTlogStatus) string {
						return meta.FindStatusCondition(status.Conditions, constants.Ready).Reason
					}, Not(Equal(constants.Error))),
				WithTransform(func(status v1alpha1.CTlogStatus) int64 {
					return status.RecoveryAttempts
				}, BeNumerically(">=", 1))))
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Error))

			By("After fixing the problem the CTlog instance is Ready")
			Eventually(func(g Gomega) error {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.TreeID = utils.Pointer(int64(1))
				return k8sClient.Update(ctx, found)
			}).Should(Succeed())

			By("Waiting until CTlog instance is Initialization")
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Initialize))

			deployments := &appsv1.DeploymentList{}
			Expect(k8sClient.List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			By("Move to Ready phase")
			for _, d := range deployments.Items {
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{Status: corev1.ConditionTrue, Type: appsv1.DeploymentAvailable, Reason: constants.Ready}}
				Expect(k8sClient.Status().Update(ctx, &d)).Should(Succeed())
			}
			// Workaround to succeed condition for Ready phase

			Eventually(func(g Gomega) bool {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}).Should(BeTrue())

			By("Pregenerated resources are reused")
			Expect(key).To(Equal(found.Status.PrivateKeyRef.Name))
		})
	})
})
