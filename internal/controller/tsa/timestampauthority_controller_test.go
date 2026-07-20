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

package tsa

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	httpmock "github.com/securesign/operator/internal/testing/http"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	httputils "github.com/securesign/operator/internal/utils/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/tsa/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/utils/ptr"
)

const testCertChainPEM = "-----BEGIN CERTIFICATE-----\nMIIBKzCB1KADAgECAgEBMAoGCCqGSM49BAMCMA8xDTALBgNVBAMTBHRlc3QwHhcN\nMjYwNjI5MTYxOTIwWhcNMjYwNjI5MTcxOTIwWjAPMQ0wCwYDVQQDEwR0ZXN0MFkw\nEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE85lB8dJ2gvU3WiSMnVW1HawG0AguTJtn\nJ0xsj1MVte14R8gbZ5JYa1JQApfaFPZASG5BDF+wtCSAxZsnv1p2nKMhMB8wHQYD\nVR0OBBYEFERbL9GJ1Bjo8NTCbJGFhPsOxZrWMAoGCCqGSM49BAMCA0YAMEMCHyCB\nX67EHWE0mk/oUaL8bXMKVVZb8nv9LVFp50xV27MCIAWXJKxRq+uzAV3KjeQHRGhV\nO6CWnSga3RNCz1GANhNf\n-----END CERTIFICATE-----"

var _ = Describe("TimestampAuthority Controller", func() {
	Context("When reconciling a resource", func() {

		const (
			Name      = "test-tsa"
			Namespace = "default"
		)

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      Name,
				Namespace: Namespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: Name, Namespace: Namespace}
		timestampAuthority := &rhtasv1.TimestampAuthority{}
		found := &rhtasv1.TimestampAuthority{}
		deployment := &appsv1.Deployment{}
		service := &corev1.Service{}
		ingress := &v1.Ingress{}

		BeforeEach(func(ctx SpecContext) {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting up HTTP mock builder for cert chain resolution")
			mockClient := &http.Client{}
			httputils.SetClientBuilder(func(_ ...[]byte) *http.Client { return mockClient })
			httpmock.SetMockTransport(mockClient, map[string]httpmock.RoundTripFunc{
				"http://tsa.localhost/api/v1/timestamp/certchain": func(_ *http.Request) *http.Response {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(testCertChainPEM)),
						Header:     make(http.Header),
					}
				},
			})
			DeferCleanup(func() {
				httputils.ResetClientBuilder()
			})
		})

		AfterEach(func(ctx SpecContext) {
			By("removing the custom resource for the Kind Timestamp Authority")
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

		It("should successfully reconcile a custom resource for the Timestamp Authority", func(ctx SpecContext) {
			By("creating the custom resource for the Timestamp Authority")
			err := suite.Client().Get(ctx, typeNamespaceName, timestampAuthority)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				tsa := &rhtasv1.TimestampAuthority{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: rhtasv1.TimestampAuthoritySpec{
						ExternalAccess: rhtasv1.ExternalAccess{
							Host:    "tsa.localhost",
							Enabled: ptr.To(true),
						},
						Monitoring: rhtasv1.MonitoringConfig{Metrics: rhtasv1.MetricsConfig{Enabled: ptr.To(false)}, ServiceMonitor: rhtasv1.ServiceMonitorConfig{Enabled: ptr.To(false)}},
						Signer: rhtasv1.TimestampAuthoritySigner{
							CertificateChain: rhtasv1.CertificateChain{
								RootCA: &rhtasv1.TsaCertificateAuthority{
									OrganizationName: "Red Hat",
								},
								IntermediateCA: []*rhtasv1.TsaCertificateAuthority{
									{
										OrganizationName: "Red Hat",
									},
								},
								LeafCA: &rhtasv1.TsaCertificateAuthority{
									OrganizationName: "Red Hat",
								},
							},
						},
						NTPMonitoring: rhtasv1.NTPMonitoring{
							Enabled: ptr.To(true),
							Config: &rhtasv1.NtpMonitoringConfig{
								RequestAttempts: 3,
								RequestTimeout:  5,
								NumServers:      4,
								ServerThreshold: 3,
								MaxTimeDelta:    6,
								Period:          60,
								Servers:         []string{"time.apple.com", "time.google.com"},
							},
						},
					},
				}
				err = suite.Client().Create(ctx, tsa)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func(ctx context.Context) error {
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).WithContext(ctx).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega, ctx context.Context) bool {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.ReadyCondition, metav1.ConditionFalse)
			}).WithContext(ctx).Should(BeTrue())

			By("Tsa signer should be resolved")
			Eventually(func(g Gomega, ctx context.Context) bool {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, actions.TSASignerCondition, metav1.ConditionTrue)
			}).WithContext(ctx).Should(BeTrue())

			By("Certificate chain secret should be created")
			Eventually(func(g Gomega, ctx context.Context) *rhtasv1.SecretKeySelector {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.CertificateChainRef
			}).WithContext(ctx).Should(Not(BeNil()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Signer.CertificateChainRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("File Signer secret should be created")
			Eventually(func(g Gomega, ctx context.Context) *rhtasv1.SecretKeySelector {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.Signer.FileSigner.PrivateKeyRef
			}).WithContext(ctx).Should(Not(BeNil()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Signer.FileSigner.PrivateKeyRef.Name, Namespace: Namespace}, &corev1.Secret{})).Should(Succeed())

			By("Should be in a creating phase")
			Eventually(func(g Gomega, ctx context.Context) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).WithContext(ctx).Should(Equal(state.Creating.String()))

			By("NTP monitoring should be resolved")
			Eventually(func(g Gomega, ctx context.Context) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).WithContext(ctx).Should(Equal(state.Initialize.String()))

			By("NTP monitoring config should be created")
			Eventually(func(g Gomega, ctx context.Context) *rhtasv1.LocalObjectReference {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.NtpConfigRef
			}).WithContext(ctx).Should(Not(BeNil()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.NtpConfigRef.Name, Namespace: Namespace}, &corev1.ConfigMap{})).Should(Succeed())

			By("Timestamp Authority service is created")
			Eventually(func(ctx context.Context) error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, service)
			}).WithContext(ctx).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).Should(Equal(int32(3000)))

			By("Checking if Ingress was successfully created in the reconciliation")
			Eventually(func(ctx context.Context) error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, ingress)
			}).WithContext(ctx).Should(Succeed())
			Expect(ingress.Spec.Rules[0].Host).Should(Equal("tsa.localhost"))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).Should(Equal(service.Name))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).Should(Equal(actions.ServerPortName))

			By("Timestamp Authority deployment is created")
			Eventually(func(ctx context.Context) error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.DeploymentName, Namespace: Namespace}, deployment)
			}).WithContext(ctx).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Waiting until Timestamp Authority instance is Ready")
			Eventually(func(g Gomega, ctx context.Context) bool {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).WithContext(ctx).Should(BeTrue())

			By("Certificate chain has been resolved into status")
			Eventually(func(g Gomega, ctx context.Context) {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				g.Expect(found.Status.CertificateChain).Should(Equal(testCertChainPEM))
			}).WithContext(ctx).Should(Succeed())

		})
	})
})
