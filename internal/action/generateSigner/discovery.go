package generateSigner

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EnsureDiscoveryLabels applies discovery labels to the target Secret and removes
// those labels from other Secrets in the namespace.
// TODO: replace discovery labels with dedicated resolve_pub_key actions that fetch
// public keys from running services.
func EnsureDiscoveryLabels(ctx context.Context, cli client.Client, namespace, secretName string, discoveryLabels map[string]string) error {
	if len(discoveryLabels) == 0 {
		return nil
	}

	// Legacy discovery labels may have different field owners, so remove them with a targeted patch.
	for label := range discoveryLabels {
		existing, err := kubernetes.ListSecrets(ctx, cli, namespace, label)
		if err != nil {
			return err
		}
		for _, s := range existing.Items {
			if s.Name == secretName {
				continue
			}
			if err := labels.Remove(ctx, &s, cli, label); err != nil {
				return err
			}
		}
	}

	secret, err := traits.Apply(corev1ac.Secret(secretName, namespace),
		traits.Labels[*corev1ac.SecretApplyConfiguration](discoveryLabels))
	if err != nil {
		return fmt.Errorf("failed to build discovery labels secret: %w", err)
	}
	// Apply directly to keep discovery ownership separate from Secret data and
	// avoid migrating legacy data ownership into this metadata-only apply.
	if err := cli.Apply(ctx, secret, client.ForceOwnership, client.FieldOwner(kubernetes.FieldManager)); err != nil {
		return fmt.Errorf("failed to apply discovery labels secret: %w", err)
	}
	return nil
}
