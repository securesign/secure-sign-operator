package kubernetes

import (
	"context"
	"fmt"

	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ExpectServiceHasAtLeastNReadyEndpoints(ctx context.Context, cli client.Client, namespace, endPointName string, endpointCount int) error {
	count := 0
	var slices discoveryv1.EndpointSliceList
	if err := cli.List(ctx, &slices, client.InNamespace(namespace), client.MatchingLabels{"app.kubernetes.io/name": endPointName}); err != nil {
		return fmt.Errorf("get endpoints for service %s/%s: %w", namespace, endPointName, err)
	}

	for _, slice := range slices.Items {
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
				count++
			}
		}
	}

	if count < endpointCount {
		return fmt.Errorf("endpoint count for endpoint: %s should equal :%v, got: %v", endPointName, endpointCount, count)
	}

	return nil
}
