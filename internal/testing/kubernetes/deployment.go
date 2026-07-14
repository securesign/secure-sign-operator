package kubernetes

import (
	"context"
	"errors"

	"github.com/securesign/operator/internal/state"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SetDeploymentToReady(ctx context.Context, cli client.Client, deployment *v1.Deployment) error {
	if deployment == nil {
		return errors.New("nil deployment")
	}

	deployment.Status.ObservedGeneration = deployment.Generation
	deployment.Status.Conditions = []v1.DeploymentCondition{
		{Status: corev1.ConditionTrue, Type: v1.DeploymentAvailable, Reason: state.Ready.String()},
		{Status: corev1.ConditionTrue, Type: v1.DeploymentProgressing, Reason: "NewReplicaSetAvailable"},
	}
	return cli.Status().Update(ctx, deployment)
}

// RolledOutDeployment builds a Deployment fixture that commonUtils.DeploymentIsRunning(ByName)
// considers fully rolled out (Available=True, Progressing=True/NewReplicaSetAvailable).
func RolledOutDeployment(name, namespace string) *v1.Deployment {
	return &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Status: v1.DeploymentStatus{
			Conditions: []v1.DeploymentCondition{
				{Type: v1.DeploymentAvailable, Status: corev1.ConditionTrue},
				{Type: v1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable"},
			},
		},
	}
}

// StalledDeployment builds a Deployment fixture that is Available but not yet (or no longer)
// finished progressing, so commonUtils.DeploymentIsRunning(ByName) reports it as not running.
func StalledDeployment(name, namespace string) *v1.Deployment {
	d := RolledOutDeployment(name, namespace)
	d.Status.Conditions[1].Status = corev1.ConditionFalse
	return d
}
