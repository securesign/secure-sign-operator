package tas

import (
	"context"
	"fmt"
	"net/http"

	"github.com/securesign/operator/test/e2e/support/tas/securesign"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	. "github.com/onsi/gomega"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/test/e2e/support"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/ctlog"
	"github.com/securesign/operator/test/e2e/support/tas/fulcio"
	"github.com/securesign/operator/test/e2e/support/tas/rekor"
	"github.com/securesign/operator/test/e2e/support/tas/trillian"
	"github.com/securesign/operator/test/e2e/support/tas/tuf"
	runtimeCli "sigs.k8s.io/controller-runtime/pkg/client"
)

func VerifyAllComponents(ctx context.Context, cli runtimeCli.Client, s *rhtasv1alpha1.Securesign, dbPresent bool) {
	trillian.Verify(ctx, cli, s.Namespace, s.Name, dbPresent)
	fulcio.Verify(ctx, cli, s.Namespace, s.Name)
	tsa.Verify(ctx, cli, s.Namespace, s.Name)
	rekor.Verify(ctx, cli, s.Namespace, s.Name, dbPresent)
	ctlog.Verify(ctx, cli, s.Namespace, s.Name)
	tuf.Verify(ctx, cli, s.Namespace, s.Name)
	securesign.Verify(ctx, cli, s.Namespace, s.Name)
}

func VerifyByCosign(ctx context.Context, cli runtimeCli.Client, s *rhtasv1alpha1.Securesign, targetImageName string) {
	f := fulcio.Get(ctx, cli, s.Namespace, s.Name)
	Expect(f).ToNot(BeNil())

	r := rekor.Get(ctx, cli, s.Namespace, s.Name)
	Expect(r).ToNot(BeNil())

	t := tuf.Get(ctx, cli, s.Namespace, s.Name)
	Expect(t).ToNot(BeNil())

	ts := tsa.Get(ctx, cli, s.Namespace, s.Name)
	Expect(ts).ToNot(BeNil())

	Eventually(func(ctx context.Context) error {
		return tsa.GetCertificateChain(ctx, cli, s.Namespace, s.Name, ts.Status.Url)
	}).WithContext(ctx).Should(Succeed())

	oidcToken, err := support.OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

	// Wait for TUF root.json to be accessible via route/ingress
	Eventually(func() error {
		resp, err := http.Get(t.Status.Url + "/root.json")
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("TUF root.json not ready: status %d", resp.StatusCode)
		}
		return nil
	}).Should(Succeed())

	Expect(clients.Execute("cosign", "initialize", "--mirror="+t.Status.Url, "--root="+t.Status.Url+"/root.json")).To(Succeed())

	Expect(clients.Execute(
		"cosign", "sign", "-y",
		"--fulcio-url="+f.Status.Url,
		"--rekor-url="+r.Status.Url,
		"--timestamp-server-url="+ts.Status.Url+"/api/v1/timestamp",
		"--oidc-issuer="+support.OidcIssuerUrl(),
		"--oidc-client-id="+support.OidcClientID(),
		"--identity-token="+oidcToken,
		targetImageName,
	)).To(Succeed())

	Expect(clients.Execute(
		"cosign", "verify",
		"--rekor-url="+r.Status.Url,
		"--timestamp-certificate-chain=ts_chain.pem",
		"--certificate-identity-regexp", ".*@redhat",
		"--certificate-oidc-issuer-regexp", ".*keycloak.*",
		targetImageName,
	)).To(Succeed())
}
