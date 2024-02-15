package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/ctlog/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyCTLog(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(func() bool {
		instance := &v1alpha1.CTlog{}
		cli.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, instance)
		return meta.IsStatusConditionTrue(instance.Status.Conditions, constants.Ready)
	}).Should(BeTrue())

	list := &v1.PodList{}
	cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.ComponentName})
	Expect(list.Items).To(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}

func GetCTLogServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: actions.ComponentName, kubernetes.NameLabel: "ctlog"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}
