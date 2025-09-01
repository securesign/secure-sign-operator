package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RemainsFunctionalWhenOnePodDeleted(ctx SpecContext, cli client.Client, namespace, componentName string, verify func()) {
	Expect(DeleteOnePodByAppLabel(ctx, cli, namespace, componentName)).To(Succeed(), "failed to delete one pod for service %s", componentName)

	Eventually(ExpectServiceHasAtLeastNReadyEndpoints).WithContext(ctx).
		WithArguments(cli, namespace, componentName, 1).
		Should(Succeed(), "service lost all ready endpoints after a single pod deletion")

	verify()

	Eventually(ExpectServiceHasAtLeastNReadyEndpoints).WithContext(ctx).
		WithArguments(cli, namespace, componentName, 2).
		Should(Succeed(), "expected service to recover to 2 ready endpoints")
}
