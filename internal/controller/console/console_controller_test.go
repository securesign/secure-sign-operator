package console

import (
	"context"
	"time"

	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	k8sTest "github.com/securesign/operator/internal/testing/kubernetes"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/console/actions"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Console controller", func() {
	Context("Console controller test", func() {

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
		console := &rhtasv1.Console{}

		BeforeEach(func() {
			By("Creating the Namespace to perform the tests")
			err := suite.Client().Create(ctx, namespace)
			Expect(err).To(Not(HaveOccurred()))
		})

		AfterEach(func() {
			By("removing the custom resource for the Kind Console")
			found := &rhtasv1.Console{}
			err := suite.Client().Get(ctx, typeNamespaceName, found)
			Expect(err).To(Not(HaveOccurred()))

			Eventually(func() error {
				return suite.Client().Delete(context.TODO(), found)
			}, 2*time.Minute, time.Second).Should(Succeed())

			By("Deleting the Namespace to perform the tests")
			_ = suite.Client().Delete(ctx, namespace)
		})

		It("should successfully reconcile a custom resource for Console", func() {
			By("creating the custom resource for the Kind Console")
			err := suite.Client().Get(ctx, typeNamespaceName, console)
			if err != nil && errors.IsNotFound(err) {
				console := &rhtasv1.Console{
					ObjectMeta: metav1.ObjectMeta{
						Name:      Name,
						Namespace: Namespace,
					},
					Spec: rhtasv1.ConsoleSpec{},
				}
				err = suite.Client().Create(ctx, console)
				Expect(err).To(Not(HaveOccurred()))
			}

			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &rhtasv1.Console{}
				return suite.Client().Get(ctx, typeNamespaceName, found)
			}).Should(Succeed())

			By("Status conditions are initialized")
			Eventually(func(g Gomega) bool {
				found := &rhtasv1.Console{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionPresentAndEqual(found.Status.Conditions, constants.ReadyCondition, metav1.ConditionFalse)
			}).Should(BeTrue())

			By("API Deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("API Service created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("UI Deployment created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.UIDeploymentName, Namespace: Namespace}, &appsv1.Deployment{})
			}).Should(Succeed())

			By("UI Service created")
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.UIDeploymentName, Namespace: Namespace}, &corev1.Service{})
			}).Should(Succeed())

			By("Waiting until Console instance is Initialization")
			Eventually(func(g Gomega) string {
				found := &rhtasv1.Console{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				cond := meta.FindStatusCondition(found.Status.Conditions, constants.ReadyCondition)
				g.Expect(cond).ToNot(BeNil())
				return cond.Reason
			}).Should(Equal(state.Initialize.String()))

			By("Move to Ready phase")
			deployments := &appsv1.DeploymentList{}
			Expect(suite.Client().List(ctx, deployments, runtimeClient.InNamespace(Namespace))).To(Succeed())
			for _, d := range deployments.Items {
				Expect(k8sTest.SetDeploymentToReady(ctx, suite.Client(), &d)).To(Succeed())
			}

			By("Waiting until Console instance is Ready")
			Eventually(func(g Gomega) bool {
				found := &rhtasv1.Console{}
				g.Expect(suite.Client().Get(ctx, typeNamespaceName, found)).Should(Succeed())
				return meta.IsStatusConditionTrue(found.Status.Conditions, constants.ReadyCondition)
			}).Should(BeTrue())

			By("Checking if controller will return deployment to desired state")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, deployment)
			}).Should(Succeed())
			replicas := int32(99)
			deployment.Spec.Replicas = &replicas
			Expect(suite.Client().Status().Update(ctx, deployment)).Should(Succeed())
			Eventually(func(g Gomega) int32 {
				deployment = &appsv1.Deployment{}
				g.Expect(suite.Client().Get(ctx, types.NamespacedName{Name: actions.ApiDeploymentName, Namespace: Namespace}, deployment)).Should(Succeed())
				return *deployment.Spec.Replicas
			}).Should(Equal(int32(1)))
		})
	})
})
