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

package fulcio

import (
	"context"
	_ "embed"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	httpmock "github.com/securesign/operator/internal/testing/http"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	httputils "github.com/securesign/operator/internal/utils/http"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

//go:embed testdata/rotated_root_cert.pem
var rotatedRootCertRaw string

// rotatedRootCert has no trailing newline, matching the format ParseTrustBundle produces.
var rotatedRootCert = strings.TrimSpace(rotatedRootCertRaw)

var _ = Describe("Fulcio hot update", func() {
	Context("Fulcio hot update test", func() {

		const (
			Name      = "test-fulcio"
			Namespace = "update"
		)

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		instance := &rhtasv1.Fulcio{}

		BeforeEach(func(ctx SpecContext) {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		BeforeEach(func() {
			By("Setting up HTTP mock builder for trust bundle resolution")
			mockClient := &http.Client{}
			httputils.SetClientBuilder(func(_ ...[]byte) *http.Client {
				return mockClient
			})
			httpmock.SetMockTransport(mockClient, map[string]httpmock.RoundTripFunc{
				"http://fulcio.localhost/api/v2/trustBundle": func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(testTrustBundleJSON)),
						Header:     make(http.Header),
					}
				},
			})
			DeferCleanup(func() {
				httputils.ResetClientBuilder()
			})
		})

		AfterEach(func(ctx SpecContext) {
			By("removing the custom resource for the Kind Fulcio")
			found := &rhtasv1.Fulcio{}
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

		It("should successfully reconcile a custom resource for Fulcio", func(ctx SpecContext) {
			By("creating the custom resource for the Kind Fulcio")
			err := suite.Client().Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				instance := &rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: rhtasv1.FulcioSpec{
						ExternalAccess: rhtasv1.ExternalAccess{
							Host:    "fulcio.localhost",
							Enabled: ptr.To(true),
						},
						Config: rhtasv1.FulcioConfig{
							OIDCIssuers: []rhtasv1.OIDCIssuer{
								{
									IssuerURL: "test",
									Issuer:    "test",
									ClientID:  "test",
									Type:      "email",
								},
							},
						},
						Certificate: rhtasv1.FulcioCert{
							OrganizationName:  "MyOrg",
							OrganizationEmail: "my@email.com",
							CommonName:        "local",
						},
						Monitoring: rhtasv1.MonitoringConfig{Metrics: rhtasv1.MetricsConfig{Enabled: ptr.To(false)}, ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)}},
					},
				}
				err = suite.Client().Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func(ctx context.Context) error {
				found := &rhtasv1.Fulcio{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).WithContext(ctx).Should(Succeed())

			deployment := &appsv1.Deployment{}
			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func(ctx context.Context) error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).WithContext(ctx).Should(Succeed())

			By("Move to Ready phase")
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Waiting until Fulcio instance is Ready")
			found := &rhtasv1.Fulcio{}
			Eventually(func(g Gomega, ctx context.Context) bool {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).WithContext(ctx).Should(BeTrue())

			By("Verify cert condition is resolved")
			Eventually(func(g Gomega, ctx context.Context) bool {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, actions.CertCondition)
			}).WithContext(ctx).Should(BeTrue())

			By("Root certificate has been resolved into status")
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.CertificateChain).Should(Equal(expectedRootCert))
			}).WithContext(ctx).Should(Succeed())

			By("Config update")
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())

			By("Update OIDC")
			Eventually(func(g Gomega, ctx context.Context) error {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.Config.OIDCIssuers[0] = rhtasv1.OIDCIssuer{
					IssuerURL: "fake",
					Issuer:    "fake",
					ClientID:  "fake",
					Type:      "email",
				}
				return suite.Client().Update(ctx, found)
			}).WithContext(ctx).Should(Succeed())

			By("Fulcio deployment is updated")
			Eventually(func(g Gomega, ctx context.Context) bool {
				updated := &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).WithContext(ctx).Should(BeFalse())

			By("Root certificate still present after config update")
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.CertificateChain).Should(Equal(expectedRootCert))
			}).WithContext(ctx).Should(Succeed())

			By("Snapshot deployment state before rotating the trust bundle")
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())

			rotatedTrustBundleJSON := `{"chains":[{"certificates":["` + strings.ReplaceAll(rotatedRootCert, "\n", "\\n") + `"]}]}`
			rotatedMockClient := &http.Client{}
			httputils.SetClientBuilder(func(_ ...[]byte) *http.Client {
				return rotatedMockClient
			})
			httpmock.SetMockTransport(rotatedMockClient, map[string]httpmock.RoundTripFunc{
				"http://fulcio.localhost/api/v2/trustBundle": func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(rotatedTrustBundleJSON)),
						Header:     make(http.Header),
					}
				},
			})

			By("Triggering another spec change to force re-resolution")
			Eventually(func(g Gomega, ctx context.Context) error {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.Config.OIDCIssuers[0] = rhtasv1.OIDCIssuer{
					IssuerURL: "fake2",
					Issuer:    "fake2",
					ClientID:  "fake2",
					Type:      "email",
				}
				return suite.Client().Update(ctx, found)
			}).WithContext(ctx).Should(Succeed())

			By("Fulcio deployment is updated again")
			Eventually(func(g Gomega, ctx context.Context) bool {
				updated := &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).WithContext(ctx).Should(BeFalse())

			By("Move to Ready phase")
			deployment = &appsv1.Deployment{}
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Rotated root certificate is flagged as drifted, not silently accepted")
			Eventually(func(g Gomega, ctx context.Context) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, trustmaterial.TrustMaterialCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).WithContext(ctx).Should(Equal(trustmaterial.ReasonDrifted))

			Eventually(func(g Gomega, ctx context.Context) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.CertificateChain
			}).WithContext(ctx).ShouldNot(Equal(rotatedRootCert))

			By("Acknowledging the drift")
			Eventually(func(g Gomega, ctx context.Context) error {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				if found.Annotations == nil {
					found.Annotations = map[string]string{}
				}
				found.Annotations[annotations.RefreshTrustMaterial] = "true"
				return suite.Client().Update(ctx, found)
			}).WithContext(ctx).Should(Succeed())

			By("Root certificate updated after acknowledgement")
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.CertificateChain).Should(Equal(rotatedRootCert))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
