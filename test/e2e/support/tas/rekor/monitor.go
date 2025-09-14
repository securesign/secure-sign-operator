package rekor

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func ParseMetricValue(metricsContent, metricName string) (float64, error) {
	pattern := fmt.Sprintf(`%s\s+(\d+(?:\.\d+)?)`, regexp.QuoteMeta(metricName))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(metricsContent)
	if len(matches) < 2 {
		return 0, fmt.Errorf("metric %s not found", metricName)
	}
	return strconv.ParseFloat(matches[1], 64)
}

func GetMonitorMetrics(ctx context.Context, cli client.Client, ns string, logPrefix string) (string, error) {
	monitorPod := GetMonitorPod(ctx, cli, ns)
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
	if logPrefix != "" {
		fmt.Printf("%s:\n%s\n", logPrefix, metricsString)
	}
	return metricsString, nil
}

// GetMonitorMetricValues retrieves both log_index_verification_total and log_index_verification_failure metrics
// and performs error checking internally using Gomega. Returns (verTotal, verFailure).
func GetMonitorMetricValues(ctx context.Context, cli client.Client, ns string, g Gomega) (float64, float64) {
	metricsContent, err := GetMonitorMetrics(ctx, cli, ns, "")
	g.Expect(err).ToNot(HaveOccurred(), "failed to get monitor metrics")

	verTotal, err := ParseMetricValue(metricsContent, "log_index_verification_total")
	g.Expect(err).ToNot(HaveOccurred(), "failed to parse log_index_verification_total")

	verFailure, err := ParseMetricValue(metricsContent, "log_index_verification_failure")
	g.Expect(err).ToNot(HaveOccurred(), "failed to parse log_index_verification_failure")

	return verTotal, verFailure
}

func GetMonitorPod(ctx context.Context, cli client.Client, ns string) *v1.Pod {
	list := &v1.PodList{}
	_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labels.LabelAppComponent: actions.MonitorComponentName})
	if len(list.Items) != 1 {
		return nil
	}
	return &list.Items[0]
}
