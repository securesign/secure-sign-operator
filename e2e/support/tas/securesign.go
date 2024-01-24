package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyClientServer(ctx context.Context, cli client.Client, namespace string) {
	Eventually(GetClientServerPod(ctx, cli, namespace)).Should(
		WithTransform(func(pod *v1.Pod) bool {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
					return true
				}
			}
			return false
		}, BeTrue()),
	)
}

func GetClientServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: controllers.ClientServerDeploymentName, kubernetes.NameLabel: controllers.ClientServerDeploymentName})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func GetSecuresign(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Securesign {
	return func() *v1alpha1.Securesign {
		instance := &v1alpha1.Securesign{}
		cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
}
