package trillian

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string, dbPresent bool) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Trillian) bool {
			return meta.IsStatusConditionTrue(f.Status.Conditions, constants.Ready)
		}, BeTrue()))

	if dbPresent {
		// trillian-db
		Eventually(func(g Gomega) (bool, error) {
			return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
				kubernetes.ComponentLabel: actions.DbComponentName,
			})
		}).Should(BeTrue())
	}

	// log server
	Eventually(func(g Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			kubernetes.ComponentLabel: actions.LogServerComponentName,
		})
	}).Should(BeTrue())

	// log signer
	Eventually(func(g Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			kubernetes.ComponentLabel: actions.LogSignerComponentName,
		})
	}).Should(BeTrue())
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Trillian {
	return func() *v1alpha1.Trillian {
		instance := &v1alpha1.Trillian{}
		Expect(cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance)).To(Succeed())
		return instance
	}
}
