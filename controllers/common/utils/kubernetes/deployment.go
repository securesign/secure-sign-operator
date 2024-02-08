package kubernetes

import (
	"context"

	v1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DeploymentIsRunning(ctx context.Context, cli client.Client, namespace string, labels map[string]string) (bool, error) {
	var err error
	list := &v1.DeploymentList{}

	if err = cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return false, err
	}
	for _, d := range list.Items {
		if d.Status.ReadyReplicas != *d.Spec.Replicas {
			return false, nil
		}
	}
	return true, nil
}

func DeploymentList(ctx context.Context, cli client.Client, component, namespace string) (*v1.DeploymentList, error) {
	var err error
	list := &v1.DeploymentList{}

	if err = cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{ComponentLabel: component}); err != nil {
		return nil, err
	}

	return list, nil
}
