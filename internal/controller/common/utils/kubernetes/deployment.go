package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrDeploymentNotReady          = errors.New("deployment not ready")
	ErrDeploymentNotObserved       = errors.New("not observed")
	ErrDeploymentNotAvailable      = errors.New("not available")
	ErrDeploymentNotFound          = errors.New("not found")
	ErrNewReplicaSetNotAvailable   = errors.New("new ReplicaSet not available")
	ErrReplicaSetRevisionNotExists = errors.New("ReplicaSet revision not exists")
)

var (
	log = ctrl.Log.WithName("deployment")
)

const (
	revisionAnnotation = "deployment.kubernetes.io/revision"
	podTemplateHash    = "pod-template-hash"
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
		revision := d.Annotations[revisionAnnotation]

		log.V(2).WithValues(
			"namespace", d.Namespace, "name",
			d.Name, "generation", d.Generation,
			"observed", d.Status.ObservedGeneration,
			"conditions", d.Status.Conditions,
			"revision", revision,
		).Info("state")

		if d.Generation != d.Status.ObservedGeneration {
			return false, fmt.Errorf("%w(%s): %w: generation %d", ErrDeploymentNotReady, d.Name, ErrDeploymentNotObserved, d.Generation)
		}

		c := getDeploymentCondition(d.Status, v1.DeploymentAvailable)
		if c == nil || c.Status != corev1.ConditionTrue {
			return false, fmt.Errorf("%w(%s): %w", ErrDeploymentNotReady, d.Name, ErrDeploymentNotAvailable)
		}

		replicaSets, err := getReplicaSets(ctx, cli, &d)
		if err != nil {
			return false, err
		}

		if revision != "" {
			var templateHash string
			for _, rs := range replicaSets {
				if rs.Annotations[revisionAnnotation] == revision {
					templateHash = rs.Labels[podTemplateHash]
				}
			}
			if templateHash == "" {
				return false, fmt.Errorf("%w(%s): %w: revision %d", ErrDeploymentNotReady, d.Name, ErrReplicaSetRevisionNotExists, d.Generation)
			}

			c = getDeploymentCondition(d.Status, v1.DeploymentProgressing)
			if c == nil || c.Status != corev1.ConditionTrue || c.Reason != "NewReplicaSetAvailable" || !strings.Contains(c.Message, templateHash) {
				return false, fmt.Errorf("%w(%s): %w", ErrDeploymentNotReady, d.Name, ErrNewReplicaSetNotAvailable)
			}
		} else {
			c = getDeploymentCondition(d.Status, v1.DeploymentProgressing)
			if c == nil || c.Status != corev1.ConditionTrue || c.Reason != "NewReplicaSetAvailable" {
				return false, fmt.Errorf("%w(%s): %w", ErrDeploymentNotReady, d.Name, ErrNewReplicaSetNotAvailable)
			}
		}
	}
	return true, nil
}

func getReplicaSets(ctx context.Context, cli client.Client, deployment *v1.Deployment) ([]v1.ReplicaSet, error) {
	list := &v1.ReplicaSetList{}
	if err := cli.List(ctx, list, client.InNamespace(deployment.Namespace)); err != nil {
		return make([]v1.ReplicaSet, 0), err
	}

	var matchedReplicaSets []v1.ReplicaSet
	for _, rs := range list.Items {
		if metav1.IsControlledBy(&rs, deployment) {
			matchedReplicaSets = append(matchedReplicaSets, rs)
		}
	}
	return matchedReplicaSets, nil
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

func FindContainerByName(dp *v1.Deployment, containerName string) *corev1.Container {
	for i, c := range dp.Spec.Template.Spec.Containers {
		if c.Name == containerName {
			return &dp.Spec.Template.Spec.Containers[i]
		}
	}
	return nil
}

func FindVolumeByName(dp *v1.Deployment, volumeName string) *corev1.Volume {
	for i, v := range dp.Spec.Template.Spec.Volumes {
		if v.Name == volumeName {
			return &dp.Spec.Template.Spec.Volumes[i]
		}
	}
	return nil
}
