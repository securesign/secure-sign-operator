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
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	httpmock "github.com/securesign/operator/internal/testing/http"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	httputils "github.com/securesign/operator/internal/utils/http"
	"k8s.io/utils/ptr"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rekor controller", func() {
	Context("Attestation configuration", func() {
		const Name = "test-attestation"
		var (
			namespace corev1.Namespace
		)

		BeforeEach(func(ctx SpecContext) {
			By("Creating the Namespace to perform the tests")
			namespace = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "rekor-attestation-",
				},
			}
			err := suite.Client().Create(ctx, &namespace)
			Expect(err).To(Not(HaveOccurred()))

			By("Setting up HTTP mock builder for public key resolution")
			httputils.SetClientBuilder(func(_ ...[]byte) *http.Client {
				return &http.Client{
					Transport: httpmock.RoundTripFunc(func(_ *http.Request) *http.Response {
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(strings.NewReader(testPubKeyPEM)),
							Header:     make(http.Header),
						}
					}),
				}
			})
			DeferCleanup(func() {
				httputils.ResetClientBuilder()
			})
		})

		AfterEach(func(ctx SpecContext) {

			By("removing the custom resource for the Kind Rekor")
			found := &rhtasv1.Rekor{}
			err := suite.Client().Get(ctx, types.NamespacedName{Name: Name, Namespace: namespace.Name}, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return suite.Client().Delete(context.TODO(), found)
			}, 2*time.Minute, time.Second).Should(Succeed())

			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, &namespace)
		})

		It("default configuration", func(ctx SpecContext) {
			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      Name,
					Namespace: namespace.Name,
				},
				Spec: rhtasv1.RekorSpec{
					TreeID: ptr.To(int64(123)),
					Monitoring: rhtasv1.MonitoringWithTLogConfig{
						MonitoringConfig: rhtasv1.MonitoringConfig{Enabled: ptr.To(false)},
					},
				},
			}

			deployAndVerify(ctx, instance)

			By("Rekor server PVC created")
			found := &rhtasv1.Rekor{}
			Eventually(func(g Gomega) string {
				g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
				return found.Status.PvcName
			}).Should(Not(BeEmpty()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.PvcName, Namespace: namespace.Name}, &corev1.PersistentVolumeClaim{})).Should(Succeed())

			deployment := &appsv1.Deployment{}
			Eventually(suite.Client().Get).WithContext(ctx).WithArguments(
				types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: namespace.Name},
				deployment,
			).Should(Succeed())

			By("Checking deployment desired state")
			Expect(findVolumeByName(deployment.Spec.Template.Spec.Volumes, "storage")).To(
				gstruct.PointTo(
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"VolumeSource": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"PersistentVolumeClaim": gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
								"ClaimName": Equal(found.Status.PvcName),
							})),
						}),
					}),
				))

			By("Checking --attestation_storage_bucket")
			Expect(deployment.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--attestation_storage_bucket", "file:///var/run/attestations?no_tmp_dir=true"))
		})

		It("file storage with BYO PVC", func(ctx SpecContext) {
			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      Name,
					Namespace: namespace.Name,
				},
				Spec: rhtasv1.RekorSpec{
					TreeID: ptr.To(int64(123)),
					Monitoring: rhtasv1.MonitoringWithTLogConfig{
						MonitoringConfig: rhtasv1.MonitoringConfig{Enabled: ptr.To(false)},
					},
					Attestations: rhtasv1.RekorAttestations{
						Pvc: rhtasv1.Pvc{
							Name: "byo-pvc",
						},
					},
				},
			}

			deployAndVerify(ctx, instance)

			By("Rekor server PVC not created")
			found := &rhtasv1.Rekor{}
			Eventually(func(g Gomega) string {
				g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
				return found.Status.PvcName
			}).Should(Not(BeEmpty()))
			Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.PvcName, Namespace: namespace.Name}, &corev1.PersistentVolumeClaim{})).Should(HaveOccurred())

			deployment := &appsv1.Deployment{}
			Eventually(suite.Client().Get).WithContext(ctx).WithArguments(
				types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: namespace.Name},
				deployment,
			).Should(Succeed())

			By("Checking deployment.volume desired state")
			Expect(findVolumeByName(deployment.Spec.Template.Spec.Volumes, "storage")).To(
				gstruct.PointTo(
					gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
						"VolumeSource": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
							"PersistentVolumeClaim": gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
								"ClaimName": Equal("byo-pvc"),
							})),
						}),
					}),
				))

			By("Checking --attestation_storage_bucket")
			Expect(deployment.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--attestation_storage_bucket", "file:///var/run/attestations?no_tmp_dir=true"))
		})

		It("memory storage", func(ctx SpecContext) {
			instance := &rhtasv1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      Name,
					Namespace: namespace.Name,
				},
				Spec: rhtasv1.RekorSpec{
					TreeID: ptr.To(int64(123)),
					Monitoring: rhtasv1.MonitoringWithTLogConfig{
						MonitoringConfig: rhtasv1.MonitoringConfig{Enabled: ptr.To(false)},
					},
					Attestations: rhtasv1.RekorAttestations{
						Enabled: ptr.To(true),
						Url:     "mem://",
					},
				},
			}

			deployAndVerify(ctx, instance)

			By("Rekor server PVC not created")
			found := &rhtasv1.Rekor{}
			Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
			Expect(found.Status.PvcName).Should(BeEmpty())

			deployment := &appsv1.Deployment{}
			Eventually(suite.Client().Get).WithContext(ctx).WithArguments(
				types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: namespace.Name},
				deployment,
			).Should(Succeed())

			By("Checking deployment.volume desired state")
			Expect(findVolumeByName(deployment.Spec.Template.Spec.Volumes, "storage")).To(BeNil())

			By("Checking --attestation_storage_bucket")
			Expect(deployment.Spec.Template.Spec.Containers[0].Args).To(ContainElements("--attestation_storage_bucket", "mem://"))
		})
	})
})

func deployAndVerify(ctx context.Context, instance *rhtasv1.Rekor) {
	GinkgoHelper()

	By("creating the custom resource for the Kind Rekor")
	Expect(suite.Client().Create(ctx, instance)).To(Not(HaveOccurred()))

	By("Checking if the custom resource was successfully created")
	Eventually(func() error {
		return suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), &rhtasv1.Rekor{})
	}).Should(Succeed())

	By("Status conditions are initialized")
	Eventually(func(g Gomega) bool {
		found := &rhtasv1.Rekor{}
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
		return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.ReadyCondition, metav1.ConditionFalse)
	}).Should(BeTrue())

	By("Rekor signer created")
	found := &rhtasv1.Rekor{}
	Eventually(func(g Gomega) *rhtasv1.SecretKeySelector {
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).To(Succeed())
		return found.Status.Signer.KeyRef
	}).Should(Not(BeNil()))
	Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Signer.KeyRef.Name, Namespace: instance.Namespace}, &corev1.Secret{})).Should(Succeed())

	By("Rekor server deployment created")
	Eventually(func() error {
		return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}, &appsv1.Deployment{})
	}).Should(Succeed())

	By("Waiting until Rekor instance is Initialization")
	Eventually(func(g Gomega) string {
		found := &rhtasv1.Rekor{}
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
		cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
		g.Expect(cond).ToNot(BeNil())
		return cond.Reason
	}).Should(Equal(state.Initialize.String()))

	By("Move to Ready phase")
	// Workaround to succeed condition for Ready phase
	deployments := &appsv1.DeploymentList{}
	Expect(suite.Client().List(ctx, deployments, client.InNamespace(instance.Namespace))).To(Succeed())
	for _, d := range deployments.Items {
		Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), &d)).To(Succeed())
	}

	By("Waiting until Rekor instance is Ready")
	Eventually(func(g Gomega) bool {
		found := &rhtasv1.Rekor{}
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
		return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
	}).Should(BeTrue())
}

func findVolumeByName(volumes []corev1.Volume, name string) *corev1.Volume {
	for _, volume := range volumes {
		if volume.Name == name {
			return &volume
		}
	}
	return nil
}
