package steps

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/securesign/operator/test/e2e/support"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Namespace = "namespace"
)

func CreateNamespace(cli client.Client, callback func(*v1.Namespace)) func(ctx ginkgo.SpecContext) {
	return func(ctx ginkgo.SpecContext) {
		namespace := support.CreateTestNamespace(ctx, cli)
		ginkgo.DeferCleanup(func(ctx ginkgo.SpecContext) {
			_ = cli.Delete(ctx, namespace)
		})
		ginkgo.DeferCleanup(DumpNamespace(cli, namespace))
		callback(namespace)
	}
}

func DumpNamespace(cli client.Client, namespace *v1.Namespace) func(ctx ginkgo.SpecContext) {
	return func(ctx ginkgo.SpecContext) {
		report := ctx.SpecReport()
		if !report.Failed() || !support.IsCIEnvironment() || namespace == nil {
			return
		}
		support.DumpNamespace(ctx, cli, namespace.Name)
	}
}
