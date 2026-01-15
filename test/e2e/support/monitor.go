package support

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/internal/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func GetMonitorMetricValues(ctx context.Context, cli client.Client, ns string, monitorComponentName string, g gomega.Gomega) (float64, float64) {
	metricsContent, err := GetMonitorMetrics(ctx, cli, ns, monitorComponentName)
	g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to get monitor metrics")

	verTotal, err := parseMetricValue(metricsContent, "log_index_verification_total")
	g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to parse log_index_verification_total")

	verFailure, err := parseMetricValue(metricsContent, "log_index_verification_failure")
	g.Expect(err).ToNot(gomega.HaveOccurred(), "failed to parse log_index_verification_failure")

	return verTotal, verFailure
}

func parseMetricValue(metricsContent, metricName string) (float64, error) {
	pattern := fmt.Sprintf(`%s\s+(\d+(?:\.\d+)?)`, regexp.QuoteMeta(metricName))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(metricsContent)
	if len(matches) < 2 {
		return 0, fmt.Errorf("metric %s not found", metricName)
	}
	return strconv.ParseFloat(matches[1], 64)
}

func GetMonitorMetrics(ctx context.Context, cli client.Client, ns string, monitorComponentName string) (string, error) {
	monitorPod := getMonitorPod(ctx, cli, ns, monitorComponentName)
	if monitorPod == nil {
		return "", fmt.Errorf("monitor pod not found in namespace %s", ns)
	}
	cfg, err := config.GetConfig()
	if err != nil {
		return "", err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", err
	}

	req := clientset.CoreV1().RESTClient().Get().
		Namespace(ns).
		Resource("pods").
		Name(monitorPod.Name).
		SubResource("proxy").
		Suffix("metrics")

	result := req.Do(ctx)
	raw, err := result.Raw()
	if err != nil {
		return "", err
	}
	metricsString := string(raw)
	return metricsString, nil
}

func getMonitorPod(ctx context.Context, cli client.Client, ns string, monitorComponentName string) *v1.Pod {
	list := &v1.PodList{}
	_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labels.LabelAppComponent: monitorComponentName})
	if len(list.Items) != 1 {
		return nil
	}
	return &list.Items[0]
}

func CreateSubtleCorruption(originalContent string) string {
	// Match base64 strings with padding "=" (likely sha256_root_hash or tree_head_signature)
	re := regexp.MustCompile(`[A-Za-z0-9+/]{40,}=`)
	firstHash := re.FindString(originalContent)
	if firstHash == "" {
		return originalContent
	}

	// Change the last character before "=" in the first hash
	if len(firstHash) < 2 || firstHash[len(firstHash)-1] != '=' {
		return originalContent
	}

	lastChar := firstHash[len(firstHash)-2 : len(firstHash)-1]
	newLastChar := "0"
	if lastChar == "0" {
		newLastChar = "1"
	}

	corruptedHash := firstHash[:len(firstHash)-2] + newLastChar + "="
	return strings.Replace(originalContent, firstHash, corruptedHash, 1)
}
