package kubernetes

import (
	"context"
	"fmt"
	"strings"

	coordv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetLeaseHolderIdentity(ctx context.Context, cli client.Client, namespace string, nameContains string) (string, error) {
	var leaseList coordv1.LeaseList
	if err := cli.List(ctx, &leaseList, client.InNamespace(namespace)); err != nil {
		return "", fmt.Errorf("listing leases failed: %w", err)
	}

	for _, l := range leaseList.Items {
		if strings.Contains(*l.Spec.HolderIdentity, nameContains) && l.Spec.HolderIdentity != nil {
			return *l.Spec.HolderIdentity, nil
		}
	}

	return "", fmt.Errorf("no leader found for component containing %q in namespace %q", nameContains, namespace)
}
