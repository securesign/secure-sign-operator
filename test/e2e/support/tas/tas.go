package tas

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	kinds = []string{
		"Securesign",
		"Trillian",
		"Fulcio",
		"Rekor",
		"CTlog",
		"Tuf",
		"TimestampAuthority",
	}
	gv = rhtasv1alpha1.GroupVersion
)

func waitForCRD(cli runtimeCli.Client, gv schema.GroupVersion, kind string) {
	Eventually(func() error {
		mapper := cli.RESTMapper()

		_, err := mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: kind}, gv.Version)
		if err != nil {
			// We must invalidate the cache so the next tick actually queries the API server.
			meta.MaybeResetRESTMapper(mapper)
			return err
		}

		return nil
	}).Should(Succeed(), "Timed out waiting for RESTMapping of %s", kind)
}

func VerifyCRDRESTEndpoints(ctx context.Context, cli runtimeCli.Client) {
	for _, kind := range kinds {
		waitForCRD(cli, gv, kind)
	}
}

func VerifyAllComponents(ctx context.Context, cli runtimeCli.Client, s *rhtasv1alpha1.Securesign, dbPresent bool) {
	trillian.Verify(ctx, cli, s.Namespace, s.Name, dbPresent)
	fulcio.Verify(ctx, cli, s.Namespace, s.Name)
	tsa.Verify(ctx, cli, s.Namespace, s.Name)
	rekor.Verify(ctx, cli, s.Namespace, s.Name, dbPresent)
	ctlog.Verify(ctx, cli, s.Namespace, s.Name)
	tuf.Verify(ctx, cli, s.Namespace, s.Name)
	securesign.Verify(ctx, cli, s.Namespace, s.Name)
}
