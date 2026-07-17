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

package tuf

import (
	"context"
	_ "embed"
	"reflect"
	"strconv"
	"time"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes"
	apilabels "k8s.io/apimachinery/pkg/labels"

	rhtasv1 "github.com/securesign/operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/networking/v1"
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

//go:embed testdata/public_key.pem
var tufTestPublicKeyPEM string

var _ = Describe("TUF controller", func() {
	Context("TUF controller test", func() {

		const (
			TufName      = "test-tuf"
			TufNamespace = "controller"
		)

		ctx := context.Background()

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: TufNamespace,
			},
		}

		typeNamespaceName := types.NamespacedName{Name: TufName, Namespace: TufNamespace}
		tuf := &rhtasv1.Tuf{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Tuf")
			found := &rhtasv1.Tuf{}
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

		It("should successfully reconcile a custom resource for Tuf", func() {
			By("creating the custom resource for the Kind Tuf")
			err := suite.Client().Get(ctx, typeNamespaceName, tuf)
			if err != nil && errors.IsNotFound(err) {
				// Let's mock our custom resource at the same way that we would
				// apply on the cluster the manifest under config/samples
				tuf := &rhtasv1.Tuf{
					ObjectMeta: metav1.ObjectMeta{
						Name:      TufName,
						Namespace: TufNamespace,
					},
					Spec: rhtasv1.TufSpec{
						ExternalAccess: rhtasv1.ExternalAccess{
							Host:    "tuf.localhost",
							Enabled: ptr.To(true),
						},
						Port: 8181,
						Keys: []rhtasv1.TufKey{
							{
								Name: "fulcio_v1.crt.pem",
								SecretRef: &rhtasv1.SecretKeySelector{
									LocalObjectReference: rhtasv1.LocalObjectReference{
										Name: "fulcio-pub-key",
									},
									Key: "cert",
								},
							},
							{
								Name: "ctfe.pub",
							},
							{
								Name: "rekor.pub",
								SecretRef: &rhtasv1.SecretKeySelector{
									LocalObjectReference: rhtasv1.LocalObjectReference{
										Name: "rekor-pub-key",
									},
									Key: "public",
								},
							},
						},
					},
				}
				err = suite.Client().Create(ctx, tuf)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &rhtasv1.Tuf{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega) bool {
				found := &rhtasv1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.ReadyCondition, metav1.ConditionFalse)
			}).Should(BeTrue())

			By("Pending phase until ctlog public key is resolved")
			Eventually(func(g Gomega) string {
				found := &rhtasv1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).Should(Equal(state.Pending.String()))

			By("Creating component CRs for autodiscovery and service URL resolution")
			ctlogCR := &rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ctlog-test",
					Namespace: typeNamespaceName.Namespace,
				},
			}
			Expect(suite.Client().Create(ctx, ctlogCR)).To(Succeed())
			ctlogCR.Status.PublicKey = tufTestPublicKeyPEM
			ctlogCR.Status.Url = "https://example.com/ctlog"
			ctlogCR.SetCondition(metav1.Condition{
				Type:    constants.ReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  state.Ready.String(),
				Message: "Component is ready",
			})
			Expect(suite.Client().Status().Update(ctx, ctlogCR)).To(Succeed())

			By("Waiting until Tuf init job is created")
			found := &rhtasv1.Tuf{}
			Eventually(func(g Gomega) string {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return found.Status.PvcName
			}).ShouldNot(BeEmpty())

			Eventually(func(g Gomega) *metav1.Condition {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, tufConstants.RepositoryCondition)
			}).Should(
				And(
					WithTransform(func(condition *metav1.Condition) string {
						return condition.Reason
					}, Equal(state.Pending.String())),
					WithTransform(func(condition *metav1.Condition) string {
						return condition.Message
					}, ContainSubstring("no items found")),
				))

			componentObjects := []utils.AddressableConditionAware{
				&rhtasv1.Fulcio{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "fulcio-test",
						Namespace: typeNamespaceName.Namespace,
					},
					Spec: rhtasv1.FulcioSpec{
						Config: rhtasv1.FulcioConfig{
							OIDCIssuers: []rhtasv1.OIDCIssuer{
								{
									ClientID: "test",
									Issuer:   "test",
									Type:     "email",
								},
							},
						},
						Certificate: rhtasv1.FulcioCert{
							CommonName:        "test",
							OrganizationName:  "test",
							OrganizationEmail: "test@test.com",
						},
					},
				},
				&rhtasv1.Rekor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rekor-test",
						Namespace: typeNamespaceName.Namespace,
					},
				},
			}
			for _, component := range componentObjects {
				Expect(suite.Client().Create(ctx, component)).To(Succeed())
			}

			Eventually(func(g Gomega) *metav1.Condition {
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.FindStatusCondition(found.Status.Conditions, tufConstants.RepositoryCondition)
			}).Should(
				And(
					WithTransform(func(condition *metav1.Condition) string {
						return condition.Reason
					}, Equal(state.Pending.String())),
					WithTransform(func(condition *metav1.Condition) string {
						return condition.Message
					}, ContainSubstring("service is not ready")),
				))

			for i, component := range componentObjects {
				Expect(setStatusURL(component, "https://example.com/"+strconv.Itoa(i))).To(BeTrue())
				component.SetCondition(metav1.Condition{
					Type:    constants.ReadyCondition,
					Status:  metav1.ConditionTrue,
					Reason:  state.Ready.String(),
					Message: "Component is ready",
				})
				Expect(suite.Client().Status().Update(ctx, component)).To(Succeed())
			}

			initJobList := &batchv1.JobList{}
			Eventually(func() []batchv1.Job {
				jobLabels := labels.ForResource(tufConstants.ComponentName, tufConstants.InitJobName, TufName, found.Status.PvcName)
				selector := apilabels.SelectorFromSet(jobLabels)
				Expect(kubernetes.FindByLabelSelector(ctx, suite.Client(), initJobList, TufNamespace, selector.String())).To(Succeed())
				return initJobList.Items
			}).Should(HaveLen(1))

			By("Move to Job to completed")
			// Workaround to succeed condition for Ready phase
			initJob := &initJobList.Items[0]
			initJob.Status.Conditions = []batchv1.JobCondition{
				{Status: corev1.ConditionTrue, Type: batchv1.JobComplete, Reason: state.Ready.String()},
				{Status: corev1.ConditionTrue, Type: batchv1.JobSuccessCriteriaMet, Reason: state.Ready.String()},
			}
			now := metav1.Now()
			initJob.Status.StartTime = &now
			initJob.Status.CompletionTime = &now

			Expect(suite.Client().Status().Update(ctx, initJob)).Should(Succeed())

			By("Repository condition gets ready")
			Eventually(func(g Gomega) bool {
				found := &rhtasv1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, tufConstants.RepositoryCondition)
			}).Should(BeTrue())

			By("Waiting until Tuf instance is Initialization")
			Eventually(func(g Gomega) string {
				found := &rhtasv1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).Should(Equal(state.Initialize.String()))

			deployment := &appsv1.Deployment{}
			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, deployment)
			}).Should(Succeed())

			By("Move to Ready phase")
			// Workaround to succeed condition for Ready phase
			Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), deployment)).To(Succeed())

			By("Waiting until Tuf instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &rhtasv1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).Should(BeTrue())

			By("Checking if Service was successfully created in the reconciliation")
			service := &corev1.Service{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, service)
			}).Should(Succeed())
			Expect(service.Spec.Ports[0].Port).Should(Equal(int32(8181)))

			By("Checking if Ingress was successfully created in the reconciliation")
			ingress := &v1.Ingress{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, ingress)
			}).Should(Succeed())
			Expect(ingress.Spec.Rules[0].Host).Should(Equal("tuf.localhost"))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Name).Should(Equal(service.Name))
			Expect(ingress.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.Service.Port.Name).Should(Equal(tufConstants.PortName))

			By("Checking the latest Status Condition added to the Tuf instance")
			Eventually(func(g Gomega) error {
				found := &rhtasv1.Tuf{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				rekorCondition := meta.FindStatusCondition(found.Status.Conditions, "rekor.pub")
				g.Expect(rekorCondition).Should(Not(BeNil()))
				g.Expect(rekorCondition.Status).Should(Equal(metav1.ConditionTrue))
				g.Expect(rekorCondition.Reason).Should(Equal(state.Ready.String()))
				ctlogCondition := meta.FindStatusCondition(found.Status.Conditions, "ctfe.pub")
				g.Expect(ctlogCondition).Should(Not(BeNil()))
				g.Expect(ctlogCondition.Status).Should(Equal(metav1.ConditionTrue))
				g.Expect(ctlogCondition.Reason).Should(Equal(state.Ready.String()))
				return nil
			}).Should(Succeed())

			By("Checking if controller will return deployment to desired state")
			deployment = &appsv1.Deployment{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, deployment)
			}).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(suite.Client().Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func(g Gomega) int32 {
				deployment = &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: tufConstants.DeploymentName, Namespace: TufNamespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}).Should(Equal(int32(1)))
		})
	})
})

func setStatusURL(obj apis.Addressable, url string) bool {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return false
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return false
	}
	status := v.FieldByName("Status")
	if !status.IsValid() || status.Kind() != reflect.Struct {
		return false
	}
	urlField := status.FieldByName("Url")
	if !urlField.IsValid() || urlField.Kind() != reflect.String || !urlField.CanSet() {
		return false
	}
	urlField.SetString(url)
	return true
}
