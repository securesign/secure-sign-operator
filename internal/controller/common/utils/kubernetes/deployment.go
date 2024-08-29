package kubernetes

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrDeploymentNotReady        = errors.New("deployment not ready")
	ErrDeploymentNotObserved     = errors.New("not observed")
	ErrDeploymentNotAvailable    = errors.New("not available")
	ErrDeploymentNotFound        = errors.New("not found")
	ErrNewReplicaSetNotAvailable = errors.New("new ReplicaSet not available")
)

var (
	log = ctrl.Log.WithName("deployment")
)

func DeploymentIsRunning(ctx context.Context, cli client.Client, namespace string, labels map[string]string) (bool, error) {
	var err error
	list := &v1.DeploymentList{}

	if err = cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return false, err
	}

	if len(list.Items) == 0 {
		return false, fmt.Errorf("%w: %w: with labels %v", ErrDeploymentNotReady, ErrDeploymentNotFound, labels)
	}

	for _, d := range list.Items {
		log.V(2).WithValues(
			"namespace", d.Namespace, "name",
			d.Name, "generation", d.Generation,
			"observed", d.Status.ObservedGeneration,
			"conditions", d.Status.Conditions,
		).Info("state")

		if d.Generation != d.Status.ObservedGeneration {
			return false, fmt.Errorf("%w(%s): %w: generation %d", ErrDeploymentNotReady, d.Name, ErrDeploymentNotObserved, d.Generation)
		}

		c := getDeploymentCondition(d.Status, v1.DeploymentAvailable)
		if c == nil || c.Status != corev1.ConditionTrue {
			return false, fmt.Errorf("%w(%s): %w", ErrDeploymentNotReady, d.Name, ErrDeploymentNotAvailable)
		}

		c = getDeploymentCondition(d.Status, v1.DeploymentProgressing)
		if c == nil || c.Status != corev1.ConditionTrue || c.Reason != "NewReplicaSetAvailable" {
			return false, fmt.Errorf("%w(%s): %w", ErrDeploymentNotReady, d.Name, ErrNewReplicaSetNotAvailable)
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
