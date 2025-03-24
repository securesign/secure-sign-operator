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

	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"

	"github.com/securesign/operator/internal/controller/rekor/actions/server"
	httpmock "github.com/securesign/operator/internal/testing/http"

	"github.com/securesign/operator/internal/controller/common/utils"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
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
			DeferCleanup(func() {
				// Ensure that we reset the DefaultClient's transport after the test
				httpmock.RestoreDefaultTransport(http.DefaultClient)
			})

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
							Enabled: utils.Pointer(false),
						},
						BackFillRedis: v1alpha1.BackFillRedis{
							Enabled: utils.Pointer(false),
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
			}).Should(Succeed())

			By("Rekor signer created")
			found := &v1alpha1.Rekor{}
			Eventually(func(g Gomega) *v1alpha1.SecretKeySelector {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).To(Succeed())
				return found.Status.Signer.KeyRef
			}).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: found.Status.Signer.KeyRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Mock http client to return public key on /api/v1/log/publicKey call")
			pubKeyData, err := kubernetes.GetSecretData(k8sClient, Namespace, &v1alpha1.SecretKeySelector{
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: found.Status.Signer.KeyRef.Name,
				},
				Key: "public",
			})
			Expect(err).To(Succeed())

			httpmock.SetMockTransport(http.DefaultClient, map[string]httpmock.RoundTripFunc{
				"http://rekor-server." + Namespace + ".svc/api/v1/log/publicKey": func(req *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader(pubKeyData)),
						Header:     make(http.Header),
					}
				},
			})

			By("Waiting until Rekor instance is Initialization")
			Eventually(func(g Gomega) string {
				found := &v1alpha1.Rekor{}
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.Ready, metav1.ConditionFalse)).Should(BeTrue())
				return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
			}).Should(Equal(constants.Initialize))

			By("Move to Ready phase")
			deployments := &appsv1.DeploymentList{}
			Expect(k8sClient.List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			for _, d := range deployments.Items {
				Expect(k8sTest.SetDeploymentToReady(ctx, k8sClient, &d)).To(Succeed())
			}
			// Workaround to succeed condition for Ready phase

			By("Rekor public key secret created")
			Eventually(func(g Gomega) {
				scr := &corev1.SecretList{}
				g.Expect(k8sClient.List(ctx, scr, runtimeClient.InNamespace(Namespace), runtimeClient.MatchingLabels{server.RekorPubLabel: "public"})).Should(Succeed())
				g.Expect(scr.Items).Should(HaveLen(1))
			}).Should(Succeed())

			By("Waiting until Rekor instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &v1alpha1.Rekor{}
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
			}).Should(BeTrue())

			By("Save the Deployment configuration")
			deployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())

			By("Patch the signer key")
			Eventually(func(g Gomega) error {
				found := &v1alpha1.Rekor{}
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.Signer.KeyRef = &v1alpha1.SecretKeySelector{
					LocalObjectReference: v1alpha1.LocalObjectReference{
						Name: "key-secret",
					},
					Key: "private",
				}
				return k8sClient.Update(ctx, found)
			}).Should(Succeed())

			By("Move to CreatingPhase by creating trillian service")
			Expect(k8sClient.Create(ctx, kubernetes.CreateSecret("key-secret", Namespace, map[string][]byte{"private": []byte("fake")}, labels.For(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)))).To(Succeed())

			httpmock.SetMockTransport(http.DefaultClient, map[string]httpmock.RoundTripFunc{
				"http://rekor-server." + Namespace + ".svc/api/v1/log/publicKey": func(req *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader([]byte("newPublicKey"))),
						Header:     make(http.Header),
					}
				},
			})

			By("Rekor deployment is updated")
			Eventually(func(g Gomega) bool {
				updated := &appsv1.Deployment{}
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).Should(BeFalse())

			By("Move to Ready phase")
			deployments = &appsv1.DeploymentList{}
			Expect(k8sClient.List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			for _, d := range deployments.Items {
				Expect(k8sTest.SetDeploymentToReady(ctx, k8sClient, &d)).To(Succeed())
			}

			By("Secret key is resolved")
			Eventually(func(g Gomega) *v1alpha1.SecretKeySelector {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.KeyRef
			}).Should(Not(BeNil()))
			Eventually(func(g Gomega) string {
				g.Expect(k8sClient.Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.KeyRef.Name
			}).Should(Equal("key-secret"))

			By("New secret with public key created")
			Eventually(func(g Gomega) {
				scr := &corev1.SecretList{}
				g.Expect(k8sClient.List(ctx, scr, runtimeClient.InNamespace(Namespace), runtimeClient.MatchingLabels{server.RekorPubLabel: "public"})).Should(Succeed())
				g.Expect(scr.Items).Should(HaveLen(1))
				g.Expect(scr.Items[0].Data).Should(HaveKeyWithValue("public", []byte("newPublicKey")))
			}).Should(Succeed())

		})
	})
})
