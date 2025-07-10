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

	"github.com/onsi/gomega/gstruct"
	"github.com/securesign/operator/internal/constants"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/utils/ptr"

	httpmock "github.com/securesign/operator/internal/testing/http"

	"github.com/securesign/operator/api/v1alpha1"
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
		})

		AfterEach(func(ctx SpecContext) {
			DeferCleanup(func() {
				// Ensure that we reset the DefaultClient's transport after the test
				httpmock.RestoreDefaultTransport(http.DefaultClient)
			})

			By("removing the custom resource for the Kind Rekor")
			found := &v1alpha1.Rekor{}
			err := suite.Client().Get(ctx, types.NamespacedName{Name: Name, Namespace: namespace.Name}, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return suite.Client().Delete(context.TODO(), found)
			}, 2*time.Minute, time.Second).Should(Succeed())

			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, &namespace)
		})

		It("default configuration", func(ctx SpecContext) {
			instance := &v1alpha1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      Name,
					Namespace: namespace.Name,
				},
				Spec: v1alpha1.RekorSpec{
					TreeID: ptr.To(int64(123)),
				},
			}

			deployAndVerify(ctx, instance)

			By("Rekor server PVC created")
			found := &v1alpha1.Rekor{}
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
			instance := &v1alpha1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      Name,
					Namespace: namespace.Name,
				},
				Spec: v1alpha1.RekorSpec{
					TreeID: ptr.To(int64(123)),
					Pvc: v1alpha1.Pvc{
						Name: "byo-pvc",
					},
				},
			}

			deployAndVerify(ctx, instance)

			By("Rekor server PVC not created")
			found := &v1alpha1.Rekor{}
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
			instance := &v1alpha1.Rekor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      Name,
					Namespace: namespace.Name,
				},
				Spec: v1alpha1.RekorSpec{
					TreeID: ptr.To(int64(123)),
					Attestations: v1alpha1.RekorAttestations{
						Enabled: ptr.To(true),
						Url:     "mem://",
					},
				},
			}

			deployAndVerify(ctx, instance)

			By("Rekor server PVC not created")
			found := &v1alpha1.Rekor{}
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

func deployAndVerify(ctx context.Context, instance *v1alpha1.Rekor) {
	GinkgoHelper()

	By("creating the custom resource for the Kind Rekor")
	Expect(suite.Client().Create(ctx, instance)).To(Not(HaveOccurred()))

	By("Checking if the custom resource was successfully created")
	Eventually(func() error {
		return suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), &v1alpha1.Rekor{})
	}).Should(Succeed())

	By("Status conditions are initialized")
	Eventually(func(g Gomega) bool {
		found := &v1alpha1.Rekor{}
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
		return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.Ready, metav1.ConditionFalse)
	}).Should(BeTrue())

	By("Rekor signer created")
	found := &v1alpha1.Rekor{}
	Eventually(func(g Gomega) *v1alpha1.SecretKeySelector {
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).To(Succeed())
		return found.Status.Signer.KeyRef
	}).Should(Not(BeNil()))
	Expect(suite.Client().Get(ctx, types.NamespacedName{Name: found.Status.Signer.KeyRef.Name, Namespace: instance.Namespace}, &corev1.Secret{})).Should(Succeed())

	By("Mock http client to return public key on /api/v1/log/publicKey call")
	pubKeyData, err := kubernetes.GetSecretData(suite.Client(), instance.Namespace, &v1alpha1.SecretKeySelector{
		LocalObjectReference: v1alpha1.LocalObjectReference{
			Name: found.Status.Signer.KeyRef.Name,
		},
		Key: "public",
	})
	Expect(err).To(Succeed())

	httpmock.SetMockTransport(http.DefaultClient, map[string]httpmock.RoundTripFunc{
		"http://rekor-server." + instance.Namespace + ".svc/api/v1/log/publicKey": func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewReader(pubKeyData)),
				Header:     make(http.Header),
			}
		},
	})

	By("Rekor server deployment created")
	Eventually(func() error {
		return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ServerDeploymentName, Namespace: instance.Namespace}, &appsv1.Deployment{})
	}).Should(Succeed())

	By("Waiting until Rekor instance is Initialization")
	Eventually(func(g Gomega) string {
		found := &v1alpha1.Rekor{}
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
		return meta.FindStatusCondition(found.Status.Conditions, constants.Ready).Reason
	}).Should(Equal(constants.Initialize))

	By("Move to Ready phase")
	// Workaround to succeed condition for Ready phase
	deployments := &appsv1.DeploymentList{}
	Expect(suite.Client().List(ctx, deployments, client.InNamespace(instance.Namespace))).To(Succeed())
	for _, d := range deployments.Items {
		Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), &d)).To(Succeed())
	}

	By("Waiting until Rekor instance is Ready")
	Eventually(func(g Gomega) bool {
		found := &v1alpha1.Rekor{}
		g.Expect(suite.Client().Get(ctx, client.ObjectKeyFromObject(instance), found)).Should(Succeed())
		return meta.IsStatusConditionTrue(found.Status.Conditions, constants.Ready)
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
