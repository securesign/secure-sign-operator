//go:build integration

package e2e

import (
	"context"
	"fmt"
	"io"
	"os/exec"
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
		// Shared Kubernetes client - initialized once and reused
		k8sClientset *kubernetes.Clientset
	)

	// Helper function to initialize Kubernetes clientset (called once)
	initKubernetesClient := func() {
		if k8sClientset == nil {
			cfg, err := config.GetConfig()
			Expect(err).ToNot(HaveOccurred())
			k8sClientset, err = kubernetes.NewForConfig(cfg)
			Expect(err).ToNot(HaveOccurred())
		}
	}

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

	// Helper function to get metrics from monitor pod
	getMetrics := func(logPrefix string) (string, error) {
		req := k8sClientset.CoreV1().RESTClient().Get().
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
		if logPrefix != "" {
			fmt.Printf("%s:\n%s\n", logPrefix, metricsString)
		}
		return metricsString, nil
	}

	// Helper function to get pod logs
	getPodLogs := func() (string, error) {
		req := k8sClientset.CoreV1().Pods(namespace.Name).GetLogs(rekorMonitorPod.Name, &v1.PodLogOptions{
			Container: actions.MonitorStatefulSetName,
		})

		logs, err := req.Stream(context.Background())
		if err != nil {
			return "", err
		}
		defer logs.Close()

		logBytes, err := io.ReadAll(logs)
		if err != nil {
			return "", err
		}

		return string(logBytes), nil
	}

	// Helper function to execute kubectl command on monitor pod
	execOnMonitorPod := func(command ...string) ([]byte, error) {
		fullCmd := append([]string{"kubectl", "exec", "-n", namespace.Name, rekorMonitorPod.Name,
			"-c", actions.MonitorStatefulSetName, "--"}, command...)
		cmd := exec.Command(fullCmd[0], fullCmd[1:]...)
		return cmd.Output()
	}

	// Helper function to execute kubectl command with combined output on monitor pod
	execOnMonitorPodWithOutput := func(command ...string) ([]byte, error) {
		fullCmd := append([]string{"kubectl", "exec", "-n", namespace.Name, rekorMonitorPod.Name,
			"-c", actions.MonitorStatefulSetName, "--"}, command...)
		cmd := exec.Command(fullCmd[0], fullCmd[1:]...)
		return cmd.CombinedOutput()
	}

	// Helper function to create subtle corruption in content
	createSubtleCorruption := func(originalContent string) string {
		// Try different character replacements to ensure we make a change
		corruptedContent := strings.ReplaceAll(originalContent, "a", "X")
		if corruptedContent == originalContent {
			corruptedContent = strings.ReplaceAll(originalContent, "0", "9")
		}
		if corruptedContent == originalContent {
			corruptedContent = strings.ReplaceAll(originalContent, "b", "c")
		}
		if corruptedContent == originalContent {
			// Last resort: change first character if possible
			if len(originalContent) > 0 {
				runes := []rune(originalContent)
				if runes[0] != 'X' {
					runes[0] = 'X'
					corruptedContent = string(runes)
				}
			}
		}
		return corruptedContent
	}

	// Helper function to log character-level differences between two strings
	logStringDifferences := func(original, corrupted string, originalBytes, corruptedBytes []byte) {
		fmt.Printf("\n=== TAMPERING COMPARISON ===\n")
		fmt.Printf("Original content length: %d bytes\n", len(originalBytes))
		fmt.Printf("Corrupted content length: %d bytes\n", len(corruptedBytes))
		fmt.Printf("Content completely replaced: %t\n", !strings.Contains(corrupted, original[:min(10, len(original))]) && len(original) > 10)

		// Show character-level differences
		diffCount := 0
		minLen := min(len(original), len(corrupted))

		fmt.Printf("Character differences found:\n")
		for i := 0; i < minLen && diffCount < 10; i++ {
			if original[i] != corrupted[i] {
				fmt.Printf("  Position %d: '%c' -> '%c'\n", i, original[i], corrupted[i])
				diffCount++
			}
		}
		if diffCount == 0 && len(original) != len(corrupted) {
			fmt.Printf("  Length difference only: %d vs %d bytes\n", len(original), len(corrupted))
		}
		fmt.Printf("Total character differences: %d (showing first 10)\n", diffCount)
		fmt.Printf("=== END COMPARISON ===\n\n")
	}

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
		BeforeEach(func() {
			// Initialize Kubernetes client for all tests in this describe block
			initKubernetesClient()
		})

		It("should not show Root hash consistency verified in log - the rekor monitor log is empty", func(ctx SpecContext) {
			By("Checking that monitor checks empty rekor log and does not contain consistency verification")
			Eventually(func(g Gomega) {
				logContent, err := getPodLogs()
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(strings.Contains(logContent, "Root hash consistency verified")).To(BeFalse(),
					fmt.Sprintf("Expected 'Root hash consistency verified' NOT to be in logs, but got: %s", logContent))
				g.Expect(strings.Contains(logContent, "empty log")).To(BeTrue(),
					fmt.Sprintf("Expected 'empty log' to be in logs, but got: %s", logContent))
			}, 1*time.Minute, 6*time.Second).Should(Succeed(),
				"Monitor log should be empty and not contain root hash consistency verification")
		})

		It("should report increasing log_index_verification_total and log_index_verification_failure remains at 0 in /metrics", func(ctx SpecContext) {
			var initialLogIndexTotal float64
			var logIndexFailure float64

			By("Getting initial metrics values")
			Eventually(func(g Gomega) {
				metricsContent, err := getMetrics("Metrics content")
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
				metricsContent, err := getMetrics("")
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
			var preSigningTotal float64

			By("Getting metrics before signing")
			Eventually(func(g Gomega) {
				metricsContent, err := getMetrics("Metrics content after signing")
				g.Expect(err).ToNot(HaveOccurred())

				total, err := parseMetricValue(metricsContent, "log_index_verification_total")
				g.Expect(err).ToNot(HaveOccurred())
				preSigningTotal = total

				failure, err := parseMetricValue(metricsContent, "log_index_verification_failure")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(failure).To(Equal(float64(0)), "Expected log_index_verification_failure to be 0 before signing")
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			By("Using cosign to sign an image, which creates real Rekor entries")
			tas.VerifyByCosign(ctx, cli, s, targetImageName)

			By("Verifying metrics after signing - total should increase, failures should remain 0")
			Eventually(func(g Gomega) {
				metricsContent, err := getMetrics("")
				g.Expect(err).ToNot(HaveOccurred())

				currentTotal, err := parseMetricValue(metricsContent, "log_index_verification_total")
				g.Expect(err).ToNot(HaveOccurred())

				currentFailure, err := parseMetricValue(metricsContent, "log_index_verification_failure")
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(currentTotal).To(BeNumerically(">=", preSigningTotal),
					fmt.Sprintf("Expected log_index_verification_total to be at least %f after signing, got %f", preSigningTotal, currentTotal))

				g.Expect(currentFailure).To(Equal(float64(0)),
					fmt.Sprintf("Expected log_index_verification_failure to remain 0 after signing, got %f", currentFailure))
			}, 2*time.Minute, 10*time.Second).Should(Succeed())

			By("Waiting for monitor to detect the new entries and verify consistency")
			Eventually(func(g Gomega) {
				logContent, err := getPodLogs()
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(strings.Contains(logContent, "Root hash consistency verified")).To(BeTrue(),
					fmt.Sprintf("Expected 'Root hash consistency verified' in logs after cosign signing, but got: %s", logContent))

			}, 5*time.Minute, 15*time.Second).Should(Succeed(),
				"Monitor should verify root hash consistency after cosign creates real Rekor entries")
		})

		It("should detect subtle corruption when checkpoint file hash is slightly modified", func(ctx SpecContext) {
			// This test simulates a more sophisticated attack where only a few characters
			// in the checkpoint are changed (like modifying a hash value), rather than
			// completely replacing the file. This is more realistic as attackers might
			// try to make minimal changes to avoid detection.

			var initialFailureCount float64

			By("Getting initial failure metrics before corruption")
			Eventually(func(g Gomega) {
				metricsContent, err := getMetrics("Metrics content")
				g.Expect(err).ToNot(HaveOccurred())

				failure, err := parseMetricValue(metricsContent, "log_index_verification_failure")
				g.Expect(err).ToNot(HaveOccurred())
				initialFailureCount = failure
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			By("Waiting for checkpoint file to be created by the monitor")
			Eventually(func(g Gomega) {
				output, err := execOnMonitorPodWithOutput("ls", "-la", "/data/checkpoint_log.txt")
				g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to list checkpoint file: %s", string(output)))
				g.Expect(string(output)).To(ContainSubstring("checkpoint_log.txt"), "Checkpoint file should exist")
			}, 2*time.Minute, 10*time.Second).Should(Succeed())

			By("Reading the original checkpoint file content")
			originalContent, err := execOnMonitorPod("cat", "/data/checkpoint_log.txt")
			Expect(err).ToNot(HaveOccurred(), "Should be able to read checkpoint file")
			fmt.Printf("\n=== ORIGINAL CHECKPOINT CONTENT (BEFORE TAMPERING) ===\n%s\n=== END ORIGINAL CONTENT ===\n", string(originalContent))

			By("Corrupting the checkpoint file with subtle hash modification")
			originalString := string(originalContent)
			corruptedContent := createSubtleCorruption(originalString)

			// Ensure we actually made a change
			Expect(corruptedContent).ToNot(Equal(originalString), "Should have made a change to the content")

			// Write the corrupted content back to the file
			output, err := execOnMonitorPodWithOutput("sh", "-c",
				fmt.Sprintf("printf '%%s' '%s' > /data/checkpoint_log.txt", corruptedContent))
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to corrupt checkpoint file: %s", string(output)))

			By("Verifying the file was corrupted with subtle changes")
			newContent, err := execOnMonitorPod("cat", "/data/checkpoint_log.txt")
			Expect(err).ToNot(HaveOccurred())
			Expect(string(newContent)).ToNot(Equal(string(originalContent)), "File content should be different after tampering")
			fmt.Printf("\n=== CORRUPTED CHECKPOINT CONTENT (AFTER TAMPERING) ===\n%s\n=== END CORRUPTED CONTENT ===\n", string(newContent))

			// Log the detailed comparison using helper function
			logStringDifferences(originalString, string(newContent), originalContent, newContent)

			By("Waiting for monitor to detect the corruption and increment failure metrics")
			Eventually(func(g Gomega) {
				metricsContent, err := getMetrics("")
				g.Expect(err).ToNot(HaveOccurred())

				currentFailure, err := parseMetricValue(metricsContent, "log_index_verification_failure")
				g.Expect(err).ToNot(HaveOccurred())

				g.Expect(currentFailure).To(BeNumerically(">", initialFailureCount),
					fmt.Sprintf("Expected log_index_verification_failure to increase from %f, but got %f", initialFailureCount, currentFailure))

			}, 2*time.Minute, 10*time.Second).Should(Succeed(),
				"Monitor should detect subtle checkpoint hash modifications and increment failure metric")

			By("Checking monitor logs for corruption detection messages")
			Eventually(func(g Gomega) {
				logContent, err := getPodLogs()
				g.Expect(err).ToNot(HaveOccurred())

				fmt.Printf("Monitor logs after corruption:\n%s\n", logContent)

				// Look for common error patterns that might indicate checkpoint corruption
				errorPatterns := []string{
					"error",
					"failed",
					"invalid",
					"corruption",
					"checkpoint",
				}

				foundError := false
				for _, pattern := range errorPatterns {
					if strings.Contains(strings.ToLower(logContent), pattern) {
						foundError = true
						break
					}
				}
				g.Expect(foundError).To(BeTrue(),
					fmt.Sprintf("Expected error messages in monitor logs indicating corruption detection, but got: %s", logContent))

			}, 2*time.Minute, 10*time.Second).Should(Succeed(),
				"Monitor logs should contain error messages indicating subtle corruption detection")
		})

	})
})
