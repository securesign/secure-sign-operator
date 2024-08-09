package rekor

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifySearchUI(ctx context.Context, cli client.Client, namespace string) {
	list := &v1.PodList{}
	Expect(cli.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels{kubernetes.ComponentLabel: actions.UIComponentName})).To(Succeed())
	Expect(list.Items).To(
		And(
			Not(BeEmpty()),
			HaveEach(WithTransform(func(p v1.Pod) v1.PodPhase { return p.Status.Phase }, Equal(v1.PodRunning))),
		),
	)
}
