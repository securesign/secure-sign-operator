package kubernetes

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func RemainsFunctionalWhenOnePodDeleted(ctx SpecContext, cli client.Client, namespace, serviceName string, verify func()) {
	Expect(DeleteOnePodByAppLabel(ctx, cli, namespace, serviceName)).To(Succeed(), "failed to delete one pod for service %s", serviceName)

	Eventually(func() error {
		return ExpectServiceHasAtLeastNReadyEndpoints(ctx, cli, namespace, serviceName, 1)
	}).Should(Succeed(), "service lost all ready endpoints after a single pod deletion")

	verify()

	Eventually(func() error {
		return ExpectServiceHasAtLeastNReadyEndpoints(ctx, cli, namespace, serviceName, 2)
	}).Should(Succeed(), "expected service to recover to 2 ready endpoints")
}
