package generateSigner

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
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
		for _, secret := range existing.Items {
			if secret.Name == secretName {
				continue
			}
			// The metadata-only list returns PartialObjectMetadata, so route the patch to Secret.
			secret.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
			before := secret.DeepCopy()
			delete(secret.Labels, label)
			// Removing the last label can clear the labels map, so guard against concurrent changes.
			patch := client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})
			if err := cli.Patch(ctx, &secret, patch); err != nil {
				return fmt.Errorf("unable to remove %q label from secret %s: %w", label, secret.Name, err)
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
