package rekor

import (
	"context"

	"github.com/securesign/operator/internal/controller/constants"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifySearchUI(ctx context.Context, cli client.Client, namespace string) {
	Eventually(func(g Gomega) (bool, error) {
		return kubernetes.DeploymentIsRunning(ctx, cli, namespace, map[string]string{
			constants.LabelAppComponent: actions.UIComponentName,
		})
	}).Should(BeTrue())
}
