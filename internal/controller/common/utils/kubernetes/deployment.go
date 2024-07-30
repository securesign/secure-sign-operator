package kubernetes

import (
	"context"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func DeploymentIsRunning(ctx context.Context, cli client.Client, namespace string, labels map[string]string) (bool, error) {
	var err error
	deploymentList := &v1.DeploymentList{}

	if err = cli.List(ctx, deploymentList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return false, err
	}
	for _, d := range deploymentList.Items {
		c := getDeploymentCondition(d.Status, v1.DeploymentAvailable)
		if c == nil || c.Status == corev1.ConditionFalse {
			return false, nil
		}
	}

	return true, nil
}

func getDeploymentCondition(status v1.DeploymentStatus, condType v1.DeploymentConditionType) *v1.DeploymentCondition {
	for i := range status.Conditions {
		c := status.Conditions[i]
		if c.Type == condType {
			return &c
		}
	}
	return nil
}
