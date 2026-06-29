//go:build integration

package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support"
	"github.com/securesign/operator/test/e2e/support/postgresql"
	"github.com/securesign/operator/test/e2e/support/steps"
	"github.com/securesign/operator/test/e2e/support/tas"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const NamespaceMask = "benchmark-install-%d-"

func BenchmarkInstall(b *testing.B) {
	gomega.RegisterTestingT(b)
	gomega.SetDefaultEventuallyTimeout(3 * time.Minute)
	log.SetLogger(ginkgo.GinkgoLogr)

	cli, err := support.CreateClient()
	if err != nil {
		b.Fatalf("could not create client: %v", err)
	}

	fipsEnabled := steps.IsFIPSCluster(context.Background(), cli)

	loop := func(iteration int) {
		var (
			namespaceName   string
			ctx             = context.Background()
			err             error
			targetImageName string
		)

		namespaceName, err = createNamespace(ctx, cli, iteration)
		if err != nil {
			b.Fatalf("could not create namespace: %v", err)
		}
		defer deleteNamespace(ctx, cli, namespaceName)
		defer dumpNamespace(ctx, cli, b, namespaceName)

		if fipsEnabled {
			if err := postgresql.CreateDB(ctx, cli, namespaceName, postgresql.DefaultSecretName, "fips-password"); err != nil {
				b.Fatalf("could not create postgresql: %v", err)
			}
			postgresql.WaitAndLoadSchema(ctx, cli, namespaceName)
		}

		targetImageName = support.PrepareImage(context.Background())

		b.StartTimer()
		err = installTAS(ctx, cli, namespaceName, fipsEnabled)
		b.StopTimer()

		if err != nil {
			b.Fatalf("could not install: %v", err)
		}
		s := securesign.Get(ctx, cli, namespaceName, "test")
		tas.VerifyByCosign(ctx, targetImageName, s.Status.TufStatus.Url, s.Status.FulcioStatus.Url, s.Status.RekorStatus.Url, s.Status.TSAStatus.Url)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop(i)
	}
}

func createNamespace(ctx context.Context, cli client.Client, iteration int) (string, error) {
	namespace := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf(NamespaceMask, iteration),
		},
	}
	err := cli.Create(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("creating namespace: %w", err)
	}
	return namespace.Name, nil
}

func installTAS(ctx context.Context, cli client.Client, namespace string, fipsEnabled bool) error {
	instance := securesign.Create(namespace, "test",
		securesign.ChooseDefaults(fipsEnabled, namespace),
	)

	if err := cli.Create(ctx, instance); err != nil {
		return fmt.Errorf("creating instance: %w", err)
	}

	tas.VerifyAllComponents(ctx, cli, instance, !fipsEnabled, true)

	return nil
}

func deleteNamespace(ctx context.Context, cli client.Client, namespace string) {
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	_ = cli.Delete(ctx, ns)
}

func dumpNamespace(ctx context.Context, cli client.Client, b *testing.B, namespace string) {
	if b.Failed() && support.IsCIEnvironment() {
		support.DumpNamespace(ctx, cli, namespace)
	}
}
