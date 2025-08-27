package rekor

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/test/e2e/support/condition"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifySearchUI(ctx context.Context, cli client.Client, namespace string) {
	Eventually(condition.DeploymentIsRunning).WithContext(ctx).
		WithArguments(cli, namespace, actions.UIComponentName).
		Should(BeTrue())
}
