package condition

import (
	"context"

	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsReady(f apis.ConditionsAwareObject) bool {
	if f == nil {
		return false
	}
	return meta.IsStatusConditionTrue(f.GetConditions(), constants.ReadyCondition)
}

func DeploymentIsRunning(ctx context.Context, cli client.Client, namespace, component string) (bool, error) {
	return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
		labels.LabelAppPartOf:    constants.AppName,
		labels.LabelAppComponent: component,
	})
}
