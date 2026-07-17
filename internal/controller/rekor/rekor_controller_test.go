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
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	"github.com/securesign/operator/internal/utils"

	httpmock "github.com/securesign/operator/internal/testing/http"
	httputils "github.com/securesign/operator/internal/utils/http"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const testPubKeyPEM = "-----BEGIN PUBLIC KEY-----\nMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEy5wMSNagtqLsSF+zf8gBVHm2VThGP69D\ngWyhhIm/BkemPBoD/BNq+/yvD2IjsV4unLp5Lcpv4UAGAPJHL/wm+tHD1nS4QKo/\nsXJ8Ezy1K+bM5DUEilcu4hGgQ7+RCG/H\n-----END PUBLIC KEY-----"

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
		instance := &rhtasv1.Rekor{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting up HTTP mock builder for public key resolution")
			mockClient := &http.Client{}
			httputils.SetClientBuilder(func(_ ...[]byte) *http.Client {
				return mockClient
			})
			httpmock.SetMockTransport(mockClient, map[string]httpmock.RoundTripFunc{
				"http://rekor.local/api/v1/log/publicKey": func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader([]byte(testPubKeyPEM))),
						Header:     make(http.Header),
					}
				},
			})
			DeferCleanup(func() {
				httputils.ResetClientBuilder()
			})
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Rekor")
			found := &rhtasv1.Rekor{}
			err := suite.Client().Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return suite.Client().Delete(ctx, found)
			}, 2*time.Minute, time.Second).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for Rekor", func() {
			By("creating the custom resource for the Kind Rekor")
			err := suite.Client().Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				treeID := int64(123)
				instance := &rhtasv1.Rekor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: rhtasv1.RekorSpec{
						TreeID: &treeID,
						ExternalAccess: rhtasv1.ExternalAccess{
							Enabled: ptr.To(true),
							Host:    "rekor.local",
						},
						Monitoring: rhtasv1.MonitoringWithTLogConfig{
							MonitoringConfig: rhtasv1.MonitoringConfig{Enabled: ptr.To(false)},
						},
						RekorSearchUI: rhtasv1.RekorSearchUI{
							Enabled: utils.Pointer(true),
						},
						BackFillRedis: rhtasv1.BackFillRedis{
							Enabled:  utils.Pointer(true),
							Schedule: "0 0 * * *",
						},
					},
				}
				err = suite.Client().Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &rhtasv1.Rekor{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega) bool {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.ReadyCondition, metav1.ConditionFalse)
			}).Should(BeTrue())

			By("Rekor signer created")
			found := &rhtasv1.Rekor{}
			Eventually(func(g Gomega) *rhtasv1.SecretKeySelector {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).To(Succeed())
				return found.Status.Signer.KeyRef
			}).Should(Not(BeNil()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Signer.KeyRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Rekor server PVC created")
			Eventually(func(g Gomega) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PvcName
			}).Should(Not(BeEmpty()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.PvcName, Namespace: Namespace}, &corev1.PersistentVolumeClaim{})).Should(Succeed())

			By("Rekor server SVC created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("Rekor server deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("Redis Deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.RedisDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("Redis svc created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.RedisDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("UI Deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("UI svc created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.SearchUiDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("Backfill Redis Cronjob Created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.BackfillRedisCronJobName, Namespace: Namespace}, &batchv1.CronJob{})
			}).Should(Succeed())

			By("Waiting until Rekor instance is Initialization")
			Eventually(func(g Gomega) string {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).Should(Equal(state.Initialize.String()))

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			deployments := &appsv1.DeploymentList{}
			Expect(suite.Client().List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			for _, d := range deployments.Items {
				Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), &d)).To(Succeed())
			}

			By("Public key status has been resolved")
			Eventually(func(g Gomega) {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).To(Succeed())
				g.Expect(found.Status.PublicKey).Should(Equal(testPubKeyPEM))
			}).Should(Succeed())

			By("Waiting until Rekor instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).Should(BeTrue())

			By("Checking if controller will return deployment to desired state")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(suite.Client().Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func(g Gomega) int32 {
				deployment = &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}).Should(Equal(int32(1)))
		})
	})
})
