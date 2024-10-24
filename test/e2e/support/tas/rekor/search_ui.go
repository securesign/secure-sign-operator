package rekor

import (
	"context"
	"github.com/securesign/operator/test/e2e/support/condition"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifySearchUI(ctx context.Context, cli client.Client, namespace string) {
	Eventually(condition.DeploymentIsRunning(ctx, cli, namespace, actions.UIComponentName)).
		Should(BeTrue())
}
