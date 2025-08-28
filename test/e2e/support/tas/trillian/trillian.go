package trillian

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/test/e2e/support/condition"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Verify(ctx context.Context, cli client.Client, namespace string, name string, dbPresent bool) {
	Eventually(Get).WithContext(ctx).WithArguments(cli, namespace, name).
		Should(
			And(
				Not(BeNil()),
				WithTransform(condition.IsReady, BeTrue()),
			))

	if dbPresent {
		// trillian-db
		Eventually(condition.DeploymentIsRunning).
			WithContext(ctx).WithArguments(cli, namespace, actions.DbComponentName).
			Should(BeTrue())
	}

	// log server
	Eventually(condition.DeploymentIsRunning).
		WithContext(ctx).WithArguments(cli, namespace, actions.LogServerComponentName).
		Should(BeTrue())

	// log signer
	Eventually(condition.DeploymentIsRunning).
		WithContext(ctx).WithArguments(cli, namespace, actions.LogSignerComponentName).
		Should(BeTrue())
}

func Get(ctx context.Context, cli client.Client, ns string, name string) *v1alpha1.Trillian {
	instance := &v1alpha1.Trillian{}
	if e := cli.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, instance); errors.IsNotFound(e) {
		return nil
	}
	return instance
}
