package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/fulcio/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyFulcio(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(GetFulcio(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Fulcio) bool {
			return meta.IsStatusConditionTrue(f.Status.Conditions, constants.Ready)
		}, BeTrue()))

	list := &v1.PodList{}
	Eventually(func(g Gomega) []v1.Pod {
		g.Expect(cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.ComponentName})).To(Succeed())
		return list.Items
	}).Should(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}

func GetFulcioServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: actions.ComponentName, kubernetes.NameLabel: "fulcio-server"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func GetFulcio(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Fulcio {
	return func() *v1alpha1.Fulcio {
		instance := &v1alpha1.Fulcio{}
		_ = cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
}
