package tas

import (
	"context"
	"fmt"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support/tas/cosign"
	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
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
	gv = rhtasv1.GroupVersion
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
	VerifyCRDRESTEndpointsForVersion(ctx, cli, gv)
}

func VerifyCRDRESTEndpointsForVersion(ctx context.Context, cli runtimeCli.Client, version schema.GroupVersion) {
	for _, kind := range kinds {
		waitForCRD(cli, version, kind)
	}
}

func VerifyAllComponents(ctx context.Context, cli runtimeCli.Client, s *rhtasv1.Securesign, dbPresent bool) {
	checks := []func(){
		func() { trillian.Verify(ctx, cli, s.Namespace, s.Name, dbPresent) },
		func() { fulcio.Verify(ctx, cli, s.Namespace, s.Name) },
		func() { tsa.Verify(ctx, cli, s.Namespace, s.Name) },
		func() { rekor.Verify(ctx, cli, s.Namespace, s.Name, dbPresent) },
		func() { ctlog.Verify(ctx, cli, s.Namespace, s.Name) },
		func() { tuf.Verify(ctx, cli, s.Namespace, s.Name) },
		func() { securesign.Verify(ctx, cli, s.Namespace, s.Name) },
	}

	// Ginkgo's Fail() panics — in a non-test goroutine that crashes the process. Recover per goroutine and re-fail on the main one.
	var wg sync.WaitGroup
	errs := make([]string, len(checks))
	wg.Add(len(checks))
	for i, check := range checks {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errs[i] = fmt.Sprintf("%v", r)
				}
			}()
			check()
		}()
	}
	wg.Wait()

	var failures []string
	for _, e := range errs {
		if e != "" {
			failures = append(failures, e)
		}
	}
	if len(failures) > 0 {
		Fail(strings.Join(failures, "\n"))
	}
}

func withPathAndCABundle(path string) OmegaMatcher {
	hasPath := WithTransform(func(w admissionregistrationv1.MutatingWebhook) string {
		return *w.ClientConfig.Service.Path
	}, Equal(path))
	hasCABundle := WithTransform(func(w admissionregistrationv1.MutatingWebhook) []byte {
		return w.ClientConfig.CABundle
	}, Not(BeEmpty()))
	return And(hasPath, hasCABundle)
}

func VerifyWebhook(ctx context.Context, cli runtimeCli.Client) {
	Eventually(func(g Gomega) {
		mwcList := &admissionregistrationv1.MutatingWebhookConfigurationList{}
		g.Expect(cli.List(ctx, mwcList)).To(Succeed())

		// Collect all webhook paths and CABundles across all MWCs.
		// Kustomize creates a single MWC with all 7 webhooks;
		// OLM creates a separate MWC per webhook definition.
		g.Expect(mwcList.Items).To(WithTransform(
			func(items []admissionregistrationv1.MutatingWebhookConfiguration) []admissionregistrationv1.MutatingWebhook {
				var all []admissionregistrationv1.MutatingWebhook
				for _, mwc := range items {
					for _, w := range mwc.Webhooks {
						if w.ClientConfig.Service != nil && w.ClientConfig.Service.Path != nil &&
							strings.HasPrefix(*w.ClientConfig.Service.Path, "/mutate-rhtas-redhat-com-") {
							all = append(all, w)
						}
					}
				}
				return all
			},
			ContainElements(
				withPathAndCABundle("/mutate-rhtas-redhat-com-v1-ctlog"),
				withPathAndCABundle("/mutate-rhtas-redhat-com-v1-fulcio"),
				withPathAndCABundle("/mutate-rhtas-redhat-com-v1-rekor"),
				withPathAndCABundle("/mutate-rhtas-redhat-com-v1-securesign"),
				withPathAndCABundle("/mutate-rhtas-redhat-com-v1-timestampauthority"),
				withPathAndCABundle("/mutate-rhtas-redhat-com-v1-trillian"),
				withPathAndCABundle("/mutate-rhtas-redhat-com-v1-tuf"),
			),
		))
	}).Should(Succeed())
}

func VerifyByCosign(ctx context.Context, targetImageName string, tufUrl, fulcioUrl, rekorUrl, tsaUrl string) {
	// use local cosign as default option
	cosign.NewLocalCosign(tufUrl, fulcioUrl, rekorUrl, tsaUrl).VerifyByCosign(ctx, targetImageName)
}
