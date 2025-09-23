//go:build integration

package e2e

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support"
	k8ssupport "github.com/securesign/operator/test/e2e/support/kubernetes"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Rekor Monitor Log", Ordered, func() {
	cli, _ := support.CreateClient()

	var (
		namespace           *v1.Namespace
		s                   *v1alpha1.Securesign
		signedImageName     string
		rekorMonitorPod     v1.Pod
		rekorMonitorService *v1.Service
	)

	execOnMonitorPodWithOutput := func(command ...string) ([]byte, error) {
		return k8ssupport.ExecInPodWithOutput(context.Background(),
			rekorMonitorPod.Name, actions.MonitorStatefulSetName, namespace.Name, command...)
	}

	execOnMonitorPod := func(command ...string) error {
		return k8ssupport.ExecInPod(context.Background(),
			rekorMonitorPod.Name, actions.MonitorStatefulSetName, namespace.Name, command...)
	}

	createSubtleCorruption := func(originalContent string) string {
		// example: rekor-server-56b7c74d54-7prtd - 8848740706025694979\n1\nEY8H3RdtBezjmTAF2AXq/h/TrZmEvPNQW13p1qQ0FoA=\n\n—
		re := regexp.MustCompile(`[A-Za-z0-9+/]{40,}=`)
		firstHash := re.FindString(originalContent)
		if firstHash == "" {
			return originalContent
		}

		// Change the last character before "=" in the first hash
		lastChar := firstHash[len(firstHash)-2 : len(firstHash)-1]
		var newLastChar string
		if lastChar == "0" {
			newLastChar = "1"
		} else {
			newLastChar = "0"
		}
		corruptedHash := firstHash[:len(firstHash)-2] + newLastChar + "="
		return strings.Replace(originalContent, firstHash, corruptedHash, 1)
	}

	BeforeAll(steps.CreateNamespace(cli, func(new *v1.Namespace) {
		namespace = new
	}))

	BeforeAll(func(ctx SpecContext) {
		s = securesign.Create(namespace.Name, "test",
			securesign.WithDefaults(),
			securesign.WithMonitoring(),
			func(v *v1alpha1.Securesign) {
				v.Spec.Rekor.Monitoring.TLog.Enabled = true
				v.Spec.Rekor.Monitoring.TLog.Interval = metav1.Duration{Duration: time.Second * 2}
			},
		)
	})

	BeforeAll(func(ctx SpecContext) {
		signedImageName = support.PrepareImage(ctx)
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

		It("should have rekor monitor service with correct labels for ServiceMonitor", func(ctx SpecContext) {
			By("Verifying rekor monitor service exists and has correct labels")
			Eventually(func(g Gomega) {
				serviceList := &v1.ServiceList{}
				err := cli.List(ctx, serviceList, ctrl.InNamespace(namespace.Name))
				g.Expect(err).ToNot(HaveOccurred())

				for _, svc := range serviceList.Items {
					if svc.Name == "rekor-monitor" {
						rekorMonitorService = &svc
						break
					}
				}
				g.Expect(rekorMonitorService).ToNot(BeNil(), "Should find rekor-monitor service")

				g.Expect(rekorMonitorService.Labels["app.kubernetes.io/component"]).To(Equal("rekor-monitor"),
					fmt.Sprintf("Service should have correct component label. Current labels: %v", rekorMonitorService.Labels))
				g.Expect(rekorMonitorService.Labels["app.kubernetes.io/name"]).ToNot(BeEmpty(),
					fmt.Sprintf("Service should have app.kubernetes.io/name label. Current labels: %v", rekorMonitorService.Labels))
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})
	})

	Describe("ServiceMonitor Creation", func() {
		It("should create and validate ServiceMonitor for rekor monitor", func(ctx SpecContext) {
			var serviceMonitor *unstructured.Unstructured
			defer func() {
				if serviceMonitor != nil {
					By("Cleaning up ServiceMonitor")
					err := cli.Delete(ctx, serviceMonitor)
					if err != nil {
						GinkgoLogr.Error(err, "Failed to cleanup ServiceMonitor", "name", serviceMonitor.GetName())
					}
				}
			}()

			By("Checking if ServiceMonitor CRD is available in the cluster")
			crdList := &unstructured.UnstructuredList{}
			crdList.SetAPIVersion("apiextensions.k8s.io/v1")
			crdList.SetKind("CustomResourceDefinitionList")
			err := cli.List(ctx, crdList)
			Expect(err).ToNot(HaveOccurred())

			serviceMonitorCRDFound := false
			for _, crd := range crdList.Items {
				if name, found, _ := unstructured.NestedString(crd.Object, "metadata", "name"); found && name == "servicemonitors.monitoring.coreos.com" {
					serviceMonitorCRDFound = true
					break
				}
			}

			if !serviceMonitorCRDFound {
				Skip("ServiceMonitor CRD (servicemonitors.monitoring.coreos.com) not found - Prometheus operator not installed")
			}

			By("Creating ServiceMonitor for rekor monitor")
			serviceMonitor = &unstructured.Unstructured{}
			serviceMonitor.SetAPIVersion("monitoring.coreos.com/v1")
			serviceMonitor.SetKind("ServiceMonitor")
			serviceMonitor.SetName("rekor-monitor-test-servicemonitor")
			serviceMonitor.SetNamespace(namespace.Name)
			serviceMonitor.SetLabels(map[string]string{
				"app": "rekor-monitor-test",
			})

			spec := map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"app.kubernetes.io/component": "rekor-monitor",
						"app.kubernetes.io/name":      rekorMonitorService.Labels["app.kubernetes.io/name"],
					},
				},
				"endpoints": []interface{}{
					map[string]interface{}{
						"port":     "monitor-metrics",
						"interval": "30s",
						"path":     "/metrics",
					},
				},
			}
			serviceMonitor.Object["spec"] = spec

			err = cli.Create(ctx, serviceMonitor)
			Expect(err).ToNot(HaveOccurred(), "Should be able to create ServiceMonitor")

			By("Verifying ServiceMonitor was created successfully")
			Eventually(func(g Gomega) {
				createdSM := &unstructured.Unstructured{}
				createdSM.SetAPIVersion("monitoring.coreos.com/v1")
				createdSM.SetKind("ServiceMonitor")
				err := cli.Get(ctx, ctrl.ObjectKey{Name: "rekor-monitor-test-servicemonitor", Namespace: namespace.Name}, createdSM)
				g.Expect(err).ToNot(HaveOccurred(), "ServiceMonitor should exist")
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("Verifying ServiceMonitor selector matches rekor monitor service")
			createdSM := &unstructured.Unstructured{}
			createdSM.SetAPIVersion("monitoring.coreos.com/v1")
			createdSM.SetKind("ServiceMonitor")
			err = cli.Get(ctx, ctrl.ObjectKey{Name: "rekor-monitor-test-servicemonitor", Namespace: namespace.Name}, createdSM)
			Expect(err).ToNot(HaveOccurred())

			selector, found, err := unstructured.NestedMap(createdSM.Object, "spec", "selector", "matchLabels")
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue(), "ServiceMonitor should have selector")
			Expect(selector["app.kubernetes.io/component"]).To(Equal("rekor-monitor"))
			Expect(selector["app.kubernetes.io/name"]).To(Equal(rekorMonitorService.Labels["app.kubernetes.io/name"]))
		})
	})

	Describe("Monitor Functionality", func() {
		It("should not show Root hash consistency verified in log - the rekor monitor log is empty", func(ctx SpecContext) {
			By("Checking that monitor checks empty rekor log and does not contain consistency verification")
			Eventually(func(g Gomega) {
				logContent, err := k8ssupport.GetPodLogs(ctx, rekorMonitorPod.Name, actions.MonitorStatefulSetName, namespace.Name)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(strings.Contains(logContent, "Root hash consistency verified")).To(BeFalse(),
					fmt.Sprintf("Expected 'Root hash consistency verified' NOT to be in logs, but got: %s", logContent))
				g.Expect(strings.Contains(logContent, "empty log")).To(BeTrue(),
					fmt.Sprintf("Expected 'empty log' to be in logs, but got: %s", logContent))
			}, 30*time.Second, 1*time.Second).Should(Succeed(),
				"Monitor log should be empty and not contain root hash consistency verification")
		})

		It("should report increasing log_index_verification_total and log_index_verification_failure remains at 0", func(ctx SpecContext) {
			var initialLogIndexTotal float64

			By("Getting initial metrics values")
			Eventually(func(g Gomega) {
				verTotal, verFailure := rekor.GetMonitorMetricValues(ctx, cli, namespace.Name, g)
				initialLogIndexTotal = verTotal
				g.Expect(verFailure).To(Equal(float64(0)),
					fmt.Sprintf("Expected log_index_verification_failure to be 0, got %f", verFailure))
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("Waiting for log_index_verification_total to increase")
			Eventually(func(g Gomega) {
				verTotal, verFailure := rekor.GetMonitorMetricValues(ctx, cli, namespace.Name, g)
				g.Expect(verTotal).To(BeNumerically(">", initialLogIndexTotal),
					fmt.Sprintf("Expected log_index_verification_total to increase from %f, but got %f", initialLogIndexTotal, verTotal))
				g.Expect(verFailure).To(Equal(float64(0)),
					fmt.Sprintf("Expected log_index_verification_failure to remain 0, got %f", verFailure))

			}, 30*time.Second, 1*time.Second).Should(Succeed(),
				"Metrics should show increasing verification total and zero failures")
		})

		It("should allow Prometheus to scrape Rekor monitor metrics", func(ctx SpecContext) {
			By("Testing that metrics are exposed in Prometheus format")
			Eventually(func(g Gomega) {
				rawMetricsContent, err := rekor.GetMonitorMetrics(ctx, cli, namespace.Name)
				g.Expect(err).ToNot(HaveOccurred(), "Should be able to get raw metrics content")

				// Verify it follows Prometheus text format conventions
				lines := strings.Split(rawMetricsContent, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					if strings.Contains(line, "log_index_verification_total") || strings.Contains(line, "log_index_verification_failure") {
						// Should have format: metric_name value
						parts := strings.Fields(line)
						g.Expect(parts).To(HaveLen(2),
							fmt.Sprintf("Metric line should have exactly 2 parts (name value), got: %s", line))
						g.Expect(parts[1]).To(MatchRegexp(`^\d+(\.\d+)?$`),
							fmt.Sprintf("Metric value should be numeric, got: %s", parts[1]))
					}
				}
			}, 30*time.Second, 1*time.Second).Should(Succeed())
		})

		It("should show 'Root hash consistency verified' in logs after cosign signing", func(ctx SpecContext) {
			var preSigningTotal float64

			By("Getting metrics before signing")
			Eventually(func(g Gomega) {
				verTotal, verFailure := rekor.GetMonitorMetricValues(ctx, cli, namespace.Name, g)
				preSigningTotal = verTotal
				g.Expect(verFailure).To(Equal(float64(0)), "Expected log_index_verification_failure to be 0 before signing")
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("Using cosign to sign an image")
			tas.VerifyByCosign(ctx, cli, s, signedImageName)

			By("Waiting for monitor to detect the new entries and verify consistency")
			Eventually(func(g Gomega) {
				logContent, err := k8ssupport.GetPodLogs(ctx, rekorMonitorPod.Name, actions.MonitorStatefulSetName, namespace.Name)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(strings.Contains(logContent, "Root hash consistency verified")).To(BeTrue(),
					fmt.Sprintf("Expected 'Root hash consistency verified' in logs after cosign signing, but got: %s", logContent))
			}, 1*time.Minute, 1*time.Second).Should(Succeed(),
				"Monitor should verify root hash consistency after cosign creates real Rekor entries")

			By("Verifying metrics after signing - total should increase, failures should remain 0")
			Eventually(func(g Gomega) {
				verTotal, verFailure := rekor.GetMonitorMetricValues(ctx, cli, namespace.Name, g)
				g.Expect(verTotal).To(BeNumerically(">=", preSigningTotal),
					fmt.Sprintf("Expected log_index_verification_total to be at least %f after signing, got %f", preSigningTotal, verTotal))
				g.Expect(verFailure).To(Equal(float64(0)),
					fmt.Sprintf("Expected log_index_verification_failure to remain 0 after signing, got %f", verFailure))
			}, 1*time.Minute, 1*time.Second).Should(Succeed())
		})

		It("should detect subtle corruption when checkpoint file hash is slightly modified", func(ctx SpecContext) {
			var initialFailureCount float64

			By("Getting initial failure metrics before corruption")
			Eventually(func(g Gomega) {
				_, verFailure := rekor.GetMonitorMetricValues(ctx, cli, namespace.Name, g)
				initialFailureCount = verFailure
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("Reading the original checkpoint file content")
			originalContent, err := execOnMonitorPodWithOutput("cat", "/data/checkpoint_log.txt")
			Expect(err).ToNot(HaveOccurred(), "Should be able to read checkpoint file")

			By("Corrupting the checkpoint file with subtle hash modification")
			originalString := string(originalContent)
			corruptedContent := createSubtleCorruption(originalString)
			Expect(corruptedContent).ToNot(Equal(originalString), "Should have modified the root hash")

			err = execOnMonitorPod("sh", "-c",
				fmt.Sprintf("printf '%%s' '%s' > /data/checkpoint_log.txt", corruptedContent))
			Expect(err).ToNot(HaveOccurred(), "Failed to corrupt checkpoint file")

			By("Verifying the file was corrupted with subtle changes")
			verifyContent, err := execOnMonitorPodWithOutput("cat", "/data/checkpoint_log.txt")
			Expect(err).ToNot(HaveOccurred())
			Expect(string(verifyContent)).ToNot(Equal(string(originalContent)), "File content should be different after tampering")

			By("Waiting for monitor to detect the corruption and increment failure metrics")
			Eventually(func(g Gomega) {
				_, verFailure := rekor.GetMonitorMetricValues(ctx, cli, namespace.Name, g)
				g.Expect(verFailure).To(BeNumerically(">", initialFailureCount),
					fmt.Sprintf("Expected log_index_verification_failure to increase from %f, but got %f", initialFailureCount, verFailure))
			}, 1*time.Minute, 1*time.Second).Should(Succeed(),
				"Monitor should detect subtle checkpoint hash modifications and increment failure metric")

			By("Checking monitor logs for corruption detection messages")
			Eventually(func(g Gomega) {
				logContent, err := k8ssupport.GetPodLogs(ctx, rekorMonitorPod.Name, actions.MonitorStatefulSetName, namespace.Name)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(strings.Contains(logContent, "error running consistency check")).To(BeTrue(),
					fmt.Sprintf("Expected 'error running consistency check' in monitor logs indicating corruption detection, but got: %s", logContent))
				g.Expect(strings.Contains(logContent, "failed to verify previous checkpoint")).To(BeTrue(),
					fmt.Sprintf("Expected 'failed to verify previous checkpoint' in monitor logs indicating corruption detection, but got: %s", logContent))
			}, 1*time.Minute, 1*time.Second).Should(Succeed(),
				"Monitor logs should contain error messages indicating subtle corruption detection")
		})

	})
})
