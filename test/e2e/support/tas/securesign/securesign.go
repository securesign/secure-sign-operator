package securesign

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support/condition"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(Get(ctx, cli, namespace, name)).Should(
		And(
			Not(BeNil()),
			WithTransform(condition.IsReady, BeTrue()),
		))
}

func Get(ctx context.Context, cli client.Client, ns string, name string) func() *v1alpha1.Securesign {
	return func() *v1alpha1.Securesign {
		instance := &v1alpha1.Securesign{}
		if e := cli.Get(ctx, types.NamespacedName{
			Namespace: ns,
			Name:      name,
		}, instance); errors.IsNotFound(e) {
			return nil
		}
		return instance
	}
}
