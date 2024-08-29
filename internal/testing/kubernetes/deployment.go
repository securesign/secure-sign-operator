package kubernetes

import (
	"context"
	"errors"
	"fmt"

	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"

	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SetDeploymentToReady(ctx context.Context, cli client.Client, deployment *v1.Deployment) error {
	if deployment == nil {
		return errors.New("nil deployment")
	}

	templateHash := kubernetes.ComputeHash(&deployment.Spec.Template, deployment.Status.CollisionCount)

	deployment.Status.ObservedGeneration = deployment.Generation
	deployment.Status.Conditions = []v1.DeploymentCondition{
		{Status: corev1.ConditionTrue, Type: v1.DeploymentAvailable, Reason: constants.Ready},
		{Status: corev1.ConditionTrue, Type: v1.DeploymentProgressing, Reason: "NewReplicaSetAvailable",
			Message: fmt.Sprintf("ReplicaSet \"%s-%s\" has successfully progressed.", deployment.Name, templateHash)},
	}
	return cli.Status().Update(ctx, deployment)
}
