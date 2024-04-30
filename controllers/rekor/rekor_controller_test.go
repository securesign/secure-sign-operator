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

	"github.com/securesign/operator/controllers/common/utils"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/rekor/actions"
	"github.com/securesign/operator/controllers/rekor/actions/server"
	trillian "github.com/securesign/operator/controllers/trillian/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rekor controller", func() {
	Context("Rekor controller test", func() {

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
							Enabled: true,
							Host:    "rekor.local",
						},
						RekorSearchUI: v1alpha1.RekorSearchUI{
							Enabled: utils.Pointer(true),
						},
						BackFillRedis: v1alpha1.BackFillRedis{
							Enabled:  utils.Pointer(true),
							Schedule: "0 0 * * *",
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

			By("Status conditions are initialized")
			Eventually(func() bool {
				found := &v1alpha1.Rekor{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.Ready, metav1.ConditionFalse)
			}, time.Minute, time.Second).Should(BeTrue())

			Eventually(func() string {
				found := &v1alpha1.Rekor{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}, time.Minute, time.Second).Should(Equal(constants.Pending))

			By("Move to CreatingPhase by creating trillian service")
			Expect(k8sClient.Create(ctx, kubernetes.CreateService(Namespace, trillian.LogserverDeploymentName, 8091, constants.LabelsForComponent(trillian.LogServerComponentName, instance.Name)))).To(Succeed())

			By("Rekor signer created")
			found := &v1alpha1.Rekor{}
			Eventually(func() *v1alpha1.SecretKeySelector {
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).To(Succeed())
				return found.Status.Signer.KeyRef
			}, time.Minute, time.Second).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Status.Signer.KeyRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Rekor public key secret created")
			Eventually(func() []corev1.Secret {
				scr := &corev1.SecretList{}
				Expect(k8sClient.List(ctx, scr, runtimeClient.InNamespace(Namespace), runtimeClient.MatchingLabels{server.RekorPubLabel: "public"})).Should(Succeed())
				return scr.Items
			}, time.Minute, time.Second).Should(Not(BeEmpty()))

			By("Rekor server PVC created")
			Eventually(func() string {
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PvcName
			}, time.Minute, time.Second).Should(Not(BeEmpty()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Status.PvcName, Namespace: Namespace}, &corev1.PersistentVolumeClaim{})).Should(Succeed())

			By("Rekor server SVC created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}, time.Minute, time.Second).Should(Succeed())

			By("Rekor server deployment created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}, time.Minute, time.Second).Should(Succeed())

			By("Redis Deployment created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.RedisDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}, time.Minute, time.Second).Should(Succeed())

			By("Redis svc created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.RedisDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}, time.Minute, time.Second).Should(Succeed())

			By("UI Deployment created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}, time.Minute, time.Second).Should(Succeed())

			By("UI svc created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}, time.Minute, time.Second).Should(Succeed())

			By("Backfill Redis Cronjob Created")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.BackfillRedisCronJobName, Namespace: Namespace}, &batchv1.CronJob{})
			}, time.Minute, time.Second).Should(Succeed())

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
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{Status: corev1.ConditionTrue, Type: appsv1.DeploymentAvailable, Reason: constants.Ready}}
				Expect(k8sClient.Status().Update(ctx, &d)).Should(Succeed())
			}
			// Workaround to succeed condition for Ready phase

			By("Waiting until Rekor instance is Ready")
			Eventually(func() bool {
				found := &v1alpha1.Rekor{}
				Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}, time.Minute, time.Second).Should(BeTrue())

			By("Checking if controller will return deployment to desired state")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, deployment)
			}, time.Minute, time.Second).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(k8sClient.Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func() int32 {
				deployment = &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}, time.Minute, time.Second).Should(Equal(int32(1)))
		})
	})
})
