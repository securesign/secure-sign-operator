package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/trillian"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyTrillian(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(func() v1alpha1.Phase {
		instance := &v1alpha1.Trillian{}
		cli.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, instance)
		return instance.Status.Phase
	}).Should(Equal(v1alpha1.PhaseReady))

	list := &v1.PodList{}
	cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: trillian.ComponentName})
	Expect(list.Items).To(And(Not(BeEmpty()), HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning)))))
}
