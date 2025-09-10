//go:build integration

package e2e

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var _ = Describe("Rekor Monitor Log", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace       *v1.Namespace
		s               *v1alpha1.Securesign
		targetImageName string
		rekorMonitorPod v1.Pod
	)

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithMonitoring(),
			func(v *v1alpha1.Securesign) {
				// Enable TLog monitoring with shorter interval for faster testing
				v.Spec.Rekor.Monitoring.TLog.Enabled = true
				v.Spec.Rekor.Monitoring.TLog.Interval = metav1.Duration{Duration: time.Second * 15}
			},
		)
	})

	BeforeAll(func(ctx SpecContext) {
		targetImageName = support.PrepareImage(ctx)
	})

	BeforeAll(func(ctx SpecContext) {
		Expect(cli.Create(ctx, s)).To(Succeed())
		By("Waiting for all TAS components to be ready")
		tas.VerifyAllComponents(ctx, cli, s, true)
	})

	Describe("Monitor Pod Deployment", func() {
		It("should have a running rekor-monitor pod", func(ctx SpecContext) {
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

				rekorMonitorPod = podList.Items[0]
				g.Expect(rekorMonitorPod.Status.Phase).To(Equal(v1.PodRunning))
			}).Should(Succeed())

			Expect(rekorMonitorPod.Status.ContainerStatuses).To(HaveLen(1))
			Expect(rekorMonitorPod.Status.ContainerStatuses[0].Ready).To(BeTrue())
		})

		It("should verify persistent volume is mounted at /data", func(ctx SpecContext) {
			Expect(rekorMonitorPod.Spec.Containers).To(HaveLen(1))
			rekorMonitorContainer := rekorMonitorPod.Spec.Containers[0]

			var dataVolumeMount *v1.VolumeMount
			for _, vm := range rekorMonitorContainer.VolumeMounts {
				if vm.MountPath == "/data" {
					dataVolumeMount = &vm
					break
				}
			}
			Expect(dataVolumeMount).ToNot(BeNil(), "Expected /data volume mount to be present")

			var dataVolume *v1.Volume
			for _, vol := range rekorMonitorPod.Spec.Volumes {
				if vol.Name == dataVolumeMount.Name {
					dataVolume = &vol
					break
				}
			}
			Expect(dataVolume).ToNot(BeNil(), "Expected volume for /data mount to be present")
			Expect(dataVolume.PersistentVolumeClaim).ToNot(BeNil(), "Expected /data to be backed by a PersistentVolumeClaim")
		})
	})

	Describe("Monitor Functionality", func() {
		It("should not show Root hash consistency verified in log - the rekor monitor log is empty", func(ctx SpecContext) {
			// Get Kubernetes clientset for log access
			cfg, err := config.GetConfig()
			Expect(err).ToNot(HaveOccurred())
			clientset, err := kubernetes.NewForConfig(cfg)
			Expect(err).ToNot(HaveOccurred())

			By("Checking that monitor checks empty rekor log and does not contain consistency verification")
			Eventually(func(g Gomega) {
				// Get pod logs
				req := clientset.CoreV1().Pods(namespace.Name).GetLogs(rekorMonitorPod.Name, &v1.PodLogOptions{
					Container: actions.MonitorStatefulSetName,
				})

				logs, err := req.Stream(context.Background())
				g.Expect(err).ToNot(HaveOccurred())
				defer logs.Close()

				logBytes, err := io.ReadAll(logs)
				if err != nil {
					g.Expect(err).ToNot(HaveOccurred())
				}

				logContent := string(logBytes)
				g.Expect(strings.Contains(logContent, "Root hash consistency verified")).To(BeFalse(),
					fmt.Sprintf("Expected 'Root hash consistency verified' NOT to be in logs, but got: %s", logContent))
				g.Expect(strings.Contains(logContent, "empty log")).To(BeTrue(),
					fmt.Sprintf("Expected 'empty log' to be in logs, but got: %s", logContent))
			}, 1*time.Minute, 6*time.Second).Should(Succeed(),
				"Monitor log should be empty and not contain root hash consistency verification")
		})

		It("should report increasing log_index_verification_total and log_index_verification_failure remains at 0 in /metrics", func(ctx SpecContext) {
			// Get Kubernetes clientset and config for port forwarding
			cfg, err := config.GetConfig()
			Expect(err).ToNot(HaveOccurred())
			clientset, err := kubernetes.NewForConfig(cfg)
			Expect(err).ToNot(HaveOccurred())

			// Helper function to parse metrics from Prometheus format
			parseMetricValue := func(metricsContent, metricName string) (float64, error) {
				pattern := fmt.Sprintf(`%s\s+(\d+(?:\.\d+)?)`, regexp.QuoteMeta(metricName))
				re := regexp.MustCompile(pattern)
				matches := re.FindStringSubmatch(metricsContent)
				if len(matches) < 2 {
					return 0, fmt.Errorf("metric %s not found", metricName)
				}
				return strconv.ParseFloat(matches[1], 64)
			}

			// Helper function to get metrics from pod
			getMetrics := func() (string, error) {
				req := clientset.CoreV1().RESTClient().Get().
					Namespace(namespace.Name).
					Resource("pods").
					Name(rekorMonitorPod.Name).
					SubResource("proxy").
					Suffix("metrics")

				result := req.Do(context.Background())
				raw, err := result.Raw()
				if err != nil {
					return "", err
				}
				metricsString := string(raw)
				fmt.Printf("Metrics content:\n%s\n", metricsString)
				return metricsString, nil
			}

			var initialLogIndexTotal float64
			var logIndexFailure float64

			By("Getting initial metrics values")
			Eventually(func(g Gomega) {
				metricsContent, err := getMetrics()
				g.Expect(err).ToNot(HaveOccurred())

				total, err := parseMetricValue(metricsContent, "log_index_verification_total")
				g.Expect(err).ToNot(HaveOccurred())
				initialLogIndexTotal = total

				failure, err := parseMetricValue(metricsContent, "log_index_verification_failure")
				g.Expect(err).ToNot(HaveOccurred())
				logIndexFailure = failure

				// Verify failure count is 0
				g.Expect(logIndexFailure).To(Equal(float64(0)),
					fmt.Sprintf("Expected log_index_verification_failure to be 0, got %f", logIndexFailure))
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			By("Waiting for log_index_verification_total to increase")
			Eventually(func(g Gomega) {
				metricsContent, err := getMetrics()
				g.Expect(err).ToNot(HaveOccurred())

				currentTotal, err := parseMetricValue(metricsContent, "log_index_verification_total")
				g.Expect(err).ToNot(HaveOccurred())

				currentFailure, err := parseMetricValue(metricsContent, "log_index_verification_failure")
				g.Expect(err).ToNot(HaveOccurred())

				// Verify total is increasing
				g.Expect(currentTotal).To(BeNumerically(">", initialLogIndexTotal),
					fmt.Sprintf("Expected log_index_verification_total to increase from %f, but got %f", initialLogIndexTotal, currentTotal))

				// Verify failure count remains 0
				g.Expect(currentFailure).To(Equal(float64(0)),
					fmt.Sprintf("Expected log_index_verification_failure to remain 0, got %f", currentFailure))

			}, 2*time.Minute, 10*time.Second).Should(Succeed(),
				"Metrics should show increasing verification total and zero failures")
		})

		It("should show 'Root hash consistency verified' in logs after cosign signing", func(ctx SpecContext) {
			// Get Kubernetes clientset for log access
			cfg, err := config.GetConfig()
			Expect(err).ToNot(HaveOccurred())
			clientset, err := kubernetes.NewForConfig(cfg)
			Expect(err).ToNot(HaveOccurred())

			By("Using cosign to sign an image, which creates real Rekor entries")
			tas.VerifyByCosign(ctx, cli, s, targetImageName)

			By("Waiting for monitor to detect the new entries and verify consistency")
			Eventually(func(g Gomega) {
				// Get pod logs
				req := clientset.CoreV1().Pods(namespace.Name).GetLogs(rekorMonitorPod.Name, &v1.PodLogOptions{
					Container: actions.MonitorStatefulSetName,
				})

				logs, err := req.Stream(context.Background())
				g.Expect(err).ToNot(HaveOccurred())
				defer logs.Close()

				logBytes, err := io.ReadAll(logs)
				if err != nil {
					g.Expect(err).ToNot(HaveOccurred())
				}

				logContent := string(logBytes)
				g.Expect(strings.Contains(logContent, "Root hash consistency verified")).To(BeTrue(),
					fmt.Sprintf("Expected 'Root hash consistency verified' in logs after cosign signing, but got: %s", logContent))

			}, 5*time.Minute, 15*time.Second).Should(Succeed(),
				"Monitor should verify root hash consistency after cosign creates real Rekor entries")
		})

	})
})
