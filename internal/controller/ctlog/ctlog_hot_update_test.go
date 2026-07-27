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
	_ "embed"
	"time"

	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"

	"github.com/securesign/operator/internal/controller/ctlog/utils"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

//go:embed testdata/fulcio_root_cert.pem
var ctlogUpdateTestFulcioRootCertPEM string

var _ = Describe("CTlog update test", func() {
	Context("CTlog update test", func() {

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
		instance := &rhtasv1.CTlog{}

		BeforeEach(func(ctx SpecContext) {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func(ctx SpecContext) {
			By("removing the custom resource for the Kind CTlog")
			found := &rhtasv1.CTlog{}
			err := suite.Client().Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func(ctx context.Context) error {
				return suite.Client().Delete(ctx, found)
			}, 3*time.Minute, time.Second).WithContext(ctx).Should(Succeed())

			// TODO(user): Attention if you improve this code by adding other context test you MUST
			// be aware of the current delete namespace limitations.
			// More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for CTlog", func(ctx SpecContext) {
			By("creating the custom resource for the Kind CTlog")
			err := suite.Client().Get(ctx, typeNamespaceName, instance)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				treeID := int64(1)
				instance := &rhtasv1.CTlog{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},

					Spec: rhtasv1.CTlogSpec{
						Trillian: rhtasv1.ServiceReference{
							Ref: &rhtasv1.ServiceReferenceRef{
								Namespace: Namespace,
								Name:      "test-trillian",
							},
						},
						TreeID: &treeID,
						Monitoring: rhtasv1.MonitoringWithTLogConfig{
							MonitoringConfig: rhtasv1.MonitoringConfig{Metrics: rhtasv1.MetricsConfig{Enabled: ptr.To(false)}, ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)}},
						},
					},
				}
				err = suite.Client().Create(ctx, instance)
				Expect(err).To(Not(HaveOccurred()))

			}

			By("Checking if the custom resource was successfully created")
			Eventually(func(ctx context.Context) error {
				found := &rhtasv1.CTlog{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).WithContext(ctx).Should(Succeed())

			By("Creating trillian service")
			By("Creating trillian object (ref to existing service)")
			Expect(suite.Client().Create(ctx, &rhtasv1.Trillian{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-trillian",
					Namespace: Namespace,
				},
			})).To(Succeed())

			By("Creating Fulcio CR with root certificate for autodiscovery")
			fulcioCR := &rhtasv1.Fulcio{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fulcio-test",
					Namespace: Namespace,
				},
				Spec: rhtasv1.FulcioSpec{
					Config: rhtasv1.FulcioConfig{
						OIDCIssuers: []rhtasv1.OIDCIssuer{{ClientID: "test", Issuer: "test", Type: "email"}},
					},
					Certificate: rhtasv1.FulcioCert{
						CommonName:        "test",
						OrganizationName:  "test",
						OrganizationEmail: "test@test.com",
					},
				},
			}
			Expect(suite.Client().Create(ctx, fulcioCR)).To(Succeed())
			fulcioCR.Status.CertificateChain = ctlogUpdateTestFulcioRootCertPEM
			fulcioCR.SetCondition(metav1.Condition{
				Type:   constants.ReadyCondition,
				Status: metav1.ConditionTrue,
				Reason: "Ready",
			})
			Expect(suite.Client().Status().Update(ctx, fulcioCR)).To(Succeed())

			deployment := &appsv1.Deployment{}
			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func(ctx context.Context) error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).WithContext(ctx).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Waiting until CTlog instance is ReadyCondition")
			Eventually(func(g Gomega, ctx context.Context) bool {
				found := &rhtasv1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).WithContext(ctx).Should(BeTrue())

			By("Public key has been resolved from operator-generated key")
			var originalPublicKey string
			Eventually(func(g Gomega, ctx context.Context) {
				found := &rhtasv1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.PublicKey).ShouldNot(BeEmpty())
				originalPublicKey = found.Status.PublicKey
			}).WithContext(ctx).Should(Succeed())

			By("Private key has changed")
			key, err := utils.CreatePrivateKey()
			Expect(err).To(Not(HaveOccurred()))
			Expect(suite.Client().Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "key-secret",
					Namespace: Namespace,
					Labels:    labels.For(actions.ComponentName, Name, instance.Name),
				},
				Data: map[string][]byte{"private": key.PrivateKey, "public": key.PublicKey},
			})).To(Succeed())

			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())
			found := &rhtasv1.CTlog{}
			Eventually(func(g Gomega, ctx context.Context) error {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				found.Spec.PrivateKeyRef = &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "key-secret",
					},
					Key: "private",
				}
				found.Spec.PublicKeyRef = &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{
						Name: "key-secret",
					},
					Key: "public",
				}
				found.Spec.PrivateKeyPasswordRef = nil //nolint:staticcheck
				return suite.Client().Update(ctx, found)
			}).WithContext(ctx).Should(Succeed())

			By("CTLog status field changed")
			Eventually(func(g Gomega, ctx context.Context) string {
				found := &rhtasv1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PrivateKeyRef.Name
			}).WithContext(ctx).Should(Equal("key-secret"))

			By("CTL deployment is updated")
			Eventually(func(g Gomega, ctx context.Context) bool {
				updated := &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, updated)).To(Succeed())
				return equality.Semantic.DeepDerivative(deployment.Spec.Template.Spec.Volumes, updated.Spec.Template.Spec.Volumes)
			}).WithContext(ctx).Should(BeFalse())

			By("Simulate deployment controller: mark updated deployment as ready")
			deployment = &appsv1.Deployment{}
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)).To(Succeed())
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Rotated public key is flagged as drifted, not silently accepted")
			Eventually(func(g Gomega, ctx context.Context) string {
				found := &rhtasv1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, trustmaterial.TrustMaterialCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).WithContext(ctx).Should(Equal(trustmaterial.ReasonDrifted))

			Eventually(func(g Gomega, ctx context.Context) string {
				found := &rhtasv1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PublicKey
			}).WithContext(ctx).Should(Equal(originalPublicKey))

			By("Acknowledging the drift")
			Eventually(func(g Gomega, ctx context.Context) error {
				found := &rhtasv1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				if found.Annotations == nil {
					found.Annotations = map[string]string{}
				}
				found.Annotations[annotations.RefreshTrustMaterial] = "true"
				return suite.Client().Update(ctx, found)
			}).WithContext(ctx).Should(Succeed())

			By("Public key status updated after key change")
			Eventually(func(g Gomega, ctx context.Context) {
				found := &rhtasv1.CTlog{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.PublicKey).ShouldNot(BeEmpty())
				g.Expect(found.Status.PublicKey).ShouldNot(Equal(originalPublicKey))
				g.Expect(found.Status.PublicKey).Should(Equal(string(key.PublicKey)))
			}).WithContext(ctx).Should(Succeed())
		})
	})
})
