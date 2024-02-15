package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifySecuresign(ctx context.Context, cli client.Client, namespace string, name string) {
	Eventually(GetSecuresign(ctx, cli, namespace, name)).Should(
		WithTransform(func(f *v1alpha1.Securesign) bool {
			return meta.IsStatusConditionTrue(f.Status.Conditions, constants.Ready)
		}, BeTrue()))
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
