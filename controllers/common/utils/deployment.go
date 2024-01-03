package utils

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DeploymentIsRunning(ctx context.Context, cli client.Client, namespace string, name string) (bool, error) {
	var err error
	d := &v1.Deployment{}

	// TODO: use object references instead hardcoded names
	if err = cli.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, d); err != nil {
		return false, fmt.Errorf("%s deployment in error state %s", name, err)
	}
	return d.Status.ReadyReplicas == *d.Spec.Replicas, nil
}
