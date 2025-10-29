package ctlog

import (
	"context"
	"fmt"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/condition"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get).WithContext(ctx).WithArguments(cli, namespace, name).
		Should(
			And(
				Not(BeNil()),
				WithTransform(condition.IsReady, BeTrue()),
			))

	Eventually(condition.DeploymentIsRunning).WithContext(ctx).
		WithArguments(cli, namespace, actions.ComponentName).
		Should(BeTrue())
}

func GetServerPod(ctx context.Context, cli client.Client, ns string) *v1.Pod {
	list := &v1.PodList{}
	_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{labels.LabelAppComponent: actions.ComponentName, labels.LabelAppName: "ctlog"})
	if len(list.Items) != 1 {
		return nil
	}
	return &list.Items[0]
}

func Get(ctx context.Context, cli client.Client, ns string, name string) *v1alpha1.CTlog {
	instance := &v1alpha1.CTlog{}
	if e := cli.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, instance); errors.IsNotFound(e) {
		return nil
	}
	return instance

}

func CreateSecret(ns string, name string) *v1.Secret {
	public, private, _, err := support.CreateCertificates(false)
	if err != nil {
		return nil
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data: map[string][]byte{
			"private": private,
			"public":  public,
		},
	}
}

// GetConfigSecret retrieves the ctlog-config secret by name
func GetConfigSecret(ctx context.Context, cli client.Client, namespace string, secretName string) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := cli.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      secretName,
	}, secret)
	return secret, err
}

// DeleteConfigSecret deletes a config secret
func DeleteConfigSecret(ctx context.Context, cli client.Client, namespace string, secretName string) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}
	return cli.Delete(ctx, secret)
}

// GetTreeIDFromStatus retrieves TreeID from CTLog status
func GetTreeIDFromStatus(ctx context.Context, cli client.Client, namespace string, name string) *int64 {
	ctlog := Get(ctx, cli, namespace, name)
	if ctlog == nil {
		return nil
	}
	return ctlog.Status.TreeID
}

// GetTrillianAddressFromSecret extracts Trillian address from config secret
func GetTrillianAddressFromSecret(secret *v1.Secret) string {
	if secret == nil {
		return ""
	}
	configData, ok := secret.Data["config"]
	if !ok {
		return ""
	}
	// Simple extraction - look for backend_spec pattern
	// In protobuf text format: backend_spec: "address:port"
	config := string(configData)
	// Return config for substring matching in tests
	return config
}

// GetTreeIDFromConfigSecret extracts TreeID from config secret
// TreeID is embedded in the protobuf config as "log_id: <number>"
func GetTreeIDFromConfigSecret(secret *v1.Secret) *int64 {
	if secret == nil {
		return nil
	}
	configData, ok := secret.Data["config"]
	if !ok {
		return nil
	}
	// Parse protobuf text format to extract log_id
	// Format: log_id: 12345 (can be indented)
	config := string(configData)

	// Split by lines and look for "log_id:" pattern
	for _, line := range splitLines(config) {
		line = trimSpace(line)
		if hasPrefix(line, "log_id:") {
			var treeID int64
			if n, _ := fmt.Sscanf(line, "log_id: %d", &treeID); n == 1 {
				return &treeID
			}
		}
	}
	return nil
}

// Helper functions for string parsing
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	// Simple trim implementation
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// VerifySecretHasNoOwnerReference checks if secret has no owner references
func VerifySecretHasNoOwnerReference(secret *v1.Secret) bool {
	return secret != nil && len(secret.OwnerReferences) == 0
}
