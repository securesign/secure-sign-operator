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

	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/annotations"
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
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rekor hot update test", func() {
	Context("Rekor update test", func() {

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
		instance := &rhtasv1.Rekor{}

		BeforeEach(func(ctx SpecContext) {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting up HTTP mock builder for public key resolution")
			mockClient := &http.Client{}
			httputils.SetClientBuilder(func(_ ...[]byte) *http.Client {
				return mockClient
			})
			httpmock.SetMockTransport(mockClient, map[string]httpmock.RoundTripFunc{
				"http://rekor-server." + Namespace + ".svc/api/v1/log/publicKey": func(_ *http.Request) *http.Response {
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

		AfterEach(func(ctx SpecContext) {
			By("removing the custom resource for the Kind Rekor")
			found := &rhtasv1.Rekor{}
			err := suite.Client().Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func(ctx context.Context) error {
				return suite.Client().Delete(ctx, found)
			}, 2*time.Minute, time.Second).WithContext(ctx).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for Rekor", func(ctx SpecContext) {
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
						Ingress: rhtasv1.Ingress{
							Enabled: ptr.To(false),
						},
						Monitoring: rhtasv1.MonitoringWithTLogConfig{
							MonitoringConfig: rhtasv1.MonitoringConfig{Metrics: rhtasv1.MetricsConfig{Enabled: ptr.To(false)}, ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)}},
						},
						RekorSearchUI: rhtasv1.RekorSearchUI{
							Enabled: utils.Pointer(false),
						},
						BackFillRedis: rhtasv1.BackFillRedis{
							Enabled: utils.Pointer(false),
						},
						Trillian: rhtasv1.ServiceReference{Ref: &rhtasv1.ServiceReferenceRef{Namespace: Namespace, Name: "test-trillian"}},
					},
				}
				err = suite.Client().Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func(ctx context.Context) error {
				found := &rhtasv1.Rekor{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).WithContext(ctx).Should(Succeed())

			By("Rekor signer created")
			found := &rhtasv1.Rekor{}
			Eventually(func(g Gomega, ctx context.Context) *rhtasv1.SecretKeySelector {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).To(Succeed())
				return found.Status.Signer.KeyRef
			}).WithContext(ctx).Should(Not(BeNil()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Signer.KeyRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Creating trillian object (needed before Rekor CR so Ref can resolve)")
			Expect(suite.Client().Create(ctx, &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trillian",
					Namespace: Namespace,
				},
			})).To(Succeed())

			By("Waiting until Rekor instance is Initialization")
			Eventually(func(g Gomega, ctx context.Context) string {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
				g.Expect(cond).ToNot(BeNil())
				g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
				return cond.Reason
			}).WithContext(ctx).Should(Equal(state.Initialize.String()))

			By("Move to Ready phase")
			deployments := &appsv1.DeploymentList{}
			Expect(suite.Client().List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			for _, d := range deployments.Items {
				Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), &d)).To(Succeed())
			}
			// Workaround to succeed condition for Ready phase

			By("Public key status resolved")
			Eventually(func(g Gomega, ctx context.Context) {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.PublicKey).Should(Equal(testPubKeyPEM))
			}).WithContext(ctx).Should(Succeed())

			By("Waiting until Rekor instance is Ready")
			Eventually(func(g Gomega, ctx context.Context) bool {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).WithContext(ctx).Should(BeTrue())

			By("Save the Deployment configuration")
			deployment := &appsv1.Deployment{}
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())

			By("Patch the signer key")
			Eventually(func(g Gomega, ctx context.Context) error {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.Signer.KeyRef = &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "key-secret",
					},
					Key: "private",
				}
				return suite.Client().Update(ctx, found)
			}).WithContext(ctx).Should(Succeed())

			By("Creating the referenced signer secret")
			Expect(suite.Client().Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "key-secret",
					Namespace: Namespace,
				},
				Data: map[string][]byte{"private": []byte("fake")},
			})).To(Succeed())

			rotatedPubKeyPEM := "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEZFt6NEqMxaeU76lnlYzFUNjFQGHq\nNF46BPCTlP/FgfMZjN608cDXf3LM5hTbvNyCEabE+4MbOcEMXhDQUlYFvA==\n-----END PUBLIC KEY-----"
			rotatedMockClient := &http.Client{}
			httputils.SetClientBuilder(func(_ ...[]byte) *http.Client {
				return rotatedMockClient
			})
			httpmock.SetMockTransport(rotatedMockClient, map[string]httpmock.RoundTripFunc{
				"http://rekor-server." + Namespace + ".svc/api/v1/log/publicKey": func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(bytes.NewReader([]byte(rotatedPubKeyPEM))),
						Header:     make(http.Header),
					}
				},
			})

			By("Rekor deployment is updated")
			Eventually(func(g Gomega, ctx context.Context) bool {
				updated := &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).WithContext(ctx).Should(BeFalse())

			By("Move to Ready phase")
			deployments = &appsv1.DeploymentList{}
			Expect(suite.Client().List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			for _, d := range deployments.Items {
				Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), &d)).To(Succeed())
			}

			By("Secret key is resolved")
			Eventually(func(g Gomega, ctx context.Context) *rhtasv1.SecretKeySelector {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.KeyRef
			}).WithContext(ctx).Should(Not(BeNil()))
			Eventually(func(g Gomega, ctx context.Context) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.KeyRef.Name
			}).WithContext(ctx).Should(Equal("key-secret"))

			By("Rotated public key is flagged as drifted, not silently accepted")
			Eventually(func(g Gomega, ctx context.Context) string {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, trustmaterial.TrustMaterialCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).WithContext(ctx).Should(Equal(trustmaterial.ReasonDrifted))

			Eventually(func(g Gomega, ctx context.Context) string {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PublicKey
			}).WithContext(ctx).ShouldNot(Equal(rotatedPubKeyPEM))

			By("Acknowledging the drift")
			Eventually(func(g Gomega, ctx context.Context) error {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				if found.Annotations == nil {
					found.Annotations = map[string]string{}
				}
				found.Annotations[annotations.RefreshTrustMaterial] = "true"
				return suite.Client().Update(ctx, found)
			}).WithContext(ctx).Should(Succeed())

			By("Public key status updated after acknowledgement")
			Eventually(func(g Gomega, ctx context.Context) {
				found := &rhtasv1.Rekor{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.PublicKey).Should(Equal(rotatedPubKeyPEM))
			}).WithContext(ctx).Should(Succeed())

		})
	})
})
