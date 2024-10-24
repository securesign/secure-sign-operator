package condition

import (
	"context"

	"github.com/onsi/gomega"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func IsReady(f apis.ConditionsAwareObject) bool {
	if f == nil {
		return false
	}
	return meta.IsStatusConditionTrue(f.GetConditions(), constants.Ready)
}

func DeploymentIsRunning(ctx context.Context, cli client.Client, namespace, component string) func(g gomega.Gomega) (bool, error) {
	return func(g gomega.Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			constants.LabelAppPartOf:    constants.AppName,
			constants.LabelAppComponent: component,
		})
	}
}
