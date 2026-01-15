//go:build integration

package e2e

import (
	"fmt"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support"
	testSupportKubernetes "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Ctlog Monitor", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace             *v1.Namespace
		ctlogMonitorPod       v1.Pod
		ctlogMonitorContainer v1.Container
		s                     *v1alpha1.Securesign
	)

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithMonitoring(),
			func(v *v1alpha1.Securesign) {
				v.Spec.Ctlog.Monitoring.TLog.Enabled = true
				v.Spec.Ctlog.Monitoring.TLog.Interval = metav1.Duration{Duration: time.Second * 10}
			},
		)
	})

	BeforeAll(func(ctx SpecContext) {
		Expect(cli.Create(ctx, s)).To(Succeed())
		By("Waiting for all TAS components to be ready")
		tas.VerifyAllComponents(ctx, cli, s, true)
	})

	Describe("Monitor Pod Deployment", func() {
		It("should have a running ctlog-monitor pod", func(ctx SpecContext) {
			By("Waiting for monitor pod to be running")
			Eventually(func(g Gomega) {
				podList := &v1.PodList{}
				err := cli.List(ctx, podList,
					ctrl.InNamespace(namespace.Name),
					ctrl.MatchingLabels{
						labels.LabelAppComponent: actions.MonitorComponentName,
					})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(1))

				ctlogMonitorPod = podList.Items[0]
				g.Expect(ctlogMonitorPod.Status.Phase).To(Equal(v1.PodRunning))
			}).Should(Succeed())

			Expect(ctlogMonitorPod.Status.ContainerStatuses).To(HaveLen(1))
			Expect(ctlogMonitorPod.Status.ContainerStatuses[0].Ready).To(BeTrue())
		})

		It("should create and run the ctlog-monitor StatefulSet", func(ctx SpecContext) {
			Eventually(func(g Gomega) {
				statefulSet := &appsv1.StatefulSet{}
				err := cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      actions.MonitorStatefulSetName,
				}, statefulSet)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(statefulSet.Status.Replicas).To(Equal(int32(1)))
				g.Expect(statefulSet.Status.ReadyReplicas).To(Equal(int32(1)))
			}).Should(Succeed())
		})

		It("should verify the Ctlog CR has monitor condition created", func(ctx SpecContext) {
			Eventually(func(g Gomega) {
				updated := &v1alpha1.CTlog{}
				err := cli.Get(ctx, types.NamespacedName{
					Namespace: namespace.Name,
					Name:      s.Name,
				}, updated)
				g.Expect(err).ToNot(HaveOccurred())

				monitorCondition := meta.FindStatusCondition(updated.Status.Conditions, actions.MonitorCondition)
				g.Expect(monitorCondition).ToNot(BeNil())
				g.Expect(monitorCondition.Type).To(Equal(actions.MonitorCondition))
				// The condition may be False with reason "Creating" - that's fine, it means monitor was created
				g.Expect(monitorCondition.Reason).To(BeElementOf([]string{"Creating", "Ready"}))
			}).Should(Succeed())
		})

		It("should verify persistent volume is mounted at /data", func(ctx SpecContext) {
			Expect(ctlogMonitorPod.Spec.Containers).To(HaveLen(1))
			ctlogMonitorContainer = ctlogMonitorPod.Spec.Containers[0]

			var dataVolumeMount *v1.VolumeMount
			for _, vm := range ctlogMonitorContainer.VolumeMounts {
				if vm.MountPath == "/data" {
					dataVolumeMount = &vm
					break
				}
			}
			Expect(dataVolumeMount).ToNot(BeNil(), "Expected /data volume mount to be present")

			var dataVolume *v1.Volume
			for _, vol := range ctlogMonitorPod.Spec.Volumes {
				if vol.Name == dataVolumeMount.Name {
					dataVolume = &vol
					break
				}
			}
			Expect(dataVolume).ToNot(BeNil(), "Expected volume for /data mount to be present")
			Expect(dataVolume.PersistentVolumeClaim).ToNot(BeNil(), "Expected /data to be backed by a PersistentVolumeClaim")
		})

		It("should verify monitor fetches checkpoints from Ctlog server using --url parameter", func(ctx SpecContext) {
			urlRegex := regexp.MustCompile(`--url=([^\s]+)`)
			var ctlogServerUrl string
			var found bool

			protocol := "http"
			if testSupportKubernetes.IsRemoteClusterOpenshift() {
				protocol = "https"
			}

			for _, cmd := range ctlogMonitorContainer.Command {
				if matches := urlRegex.FindStringSubmatch(cmd); len(matches) == 2 {
					ctlogServerUrl = matches[1]
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "Expected --url parameter to be present in container command")
			Expect(ctlogServerUrl).ToNot(BeEmpty(), "Expected URL to not be empty")

			// Verify the URL matches exactly what the ctlog service provides
			expectedCtlogServerUrl := fmt.Sprintf("%s://ctlog.%s.svc/trusted-artifact-signer", protocol, namespace.Name)
			Expect(ctlogServerUrl).To(Equal(expectedCtlogServerUrl),
				fmt.Sprintf("Expected URL to be %s, but got %s", expectedCtlogServerUrl, ctlogServerUrl))
		})
	})
})
