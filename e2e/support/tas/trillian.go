package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"github.com/securesign/operator/controllers/trillian/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyTrillian(ctx context.Context, cli client.Client, namespace string, name string, dbPresent bool) {
	Eventually(func() bool {
		instance := &v1alpha1.Trillian{}
		cli.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, instance)
		return meta.IsStatusConditionTrue(instance.Status.Conditions, constants.Ready)
	}).Should(BeTrue())

	list := &v1.PodList{}
	if dbPresent {
		Eventually(func() []v1.Pod {
			cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.DbComponentName})
			return list.Items
		}).Should(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
	}

	Eventually(func() []v1.Pod {
		cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.LogServerComponentName})
		return list.Items
	}).Should(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))

	Eventually(func() []v1.Pod {
		cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.LogSignerComponentName})
		return list.Items
	}).Should(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}
