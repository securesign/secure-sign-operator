package console

import (
	"context"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/console/actions"
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
		// console-db
		Eventually(condition.DeploymentIsRunning).
			WithContext(ctx).WithArguments(cli, namespace, actions.DbDeploymentName).
			Should(BeTrue())
	}

	// console-api
	Eventually(condition.DeploymentIsRunning).
		WithContext(ctx).WithArguments(cli, namespace, actions.ApiDeploymentName).
		Should(BeTrue())

	// console-ui
	Eventually(condition.DeploymentIsRunning).
		WithContext(ctx).WithArguments(cli, namespace, actions.UIDeploymentName).
		Should(BeTrue())
}

func Get(ctx context.Context, cli client.Client, ns string, name string) *rhtasv1.Console {
	instance := &rhtasv1.Console{}
	if e := cli.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}, instance); errors.IsNotFound(e) {
		return nil
	}
	return instance
}
