package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyRekor(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(GetRekor(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Rekor) bool {
			return meta.IsStatusConditionTrue(f.Status.Conditions, constants.Ready)
		}, BeTrue()))

	list := &v1.PodList{}

	// server
	Eventually(func(g Gomega) []v1.Pod {
		g.Expect(cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.ServerComponentName})).To(Succeed())
		return list.Items
	}).Should(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))

	// redis
	Eventually(func(g Gomega) []v1.Pod {
		g.Expect(cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.RedisComponentName})).To(Succeed())
		return list.Items
	}).Should(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}

func GetRekorServerPod(ctx context.Context, cli client.Client, ns string) func() *v1.Pod {
	return func() *v1.Pod {
		list := &v1.PodList{}
		_ = cli.List(ctx, list, client.InNamespace(ns), client.MatchingLabels{kubernetes.ComponentLabel: actions.ServerComponentName, kubernetes.NameLabel: "rekor-server"})
		if len(list.Items) != 1 {
			return nil
		}
		return &list.Items[0]
	}
}

func GetRekor(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Rekor {
	return func() *v1alpha1.Rekor {
		instance := &v1alpha1.Rekor{}
		_ = cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)
		return instance
	}
}

func VerifyRekorSearchUI(ctx context.Context, cli client.Client, namespace string, name string) {
	list := &v1.PodList{}
	Expect(cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.UIComponentName})).To(Succeed())
	Expect(list.Items).To(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}
