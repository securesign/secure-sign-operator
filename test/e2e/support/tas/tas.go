package tas

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/alexflint/go-filemutex"
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

const lockFileName = "cosign.lock"

func VerifyAllComponents(ctx context.Context, cli runtimeCli.Client, s *rhtasv1alpha1.Securesign, dbPresent bool) {
	trillian.Verify(ctx, cli, s.Namespace, s.Name, dbPresent)
	fulcio.Verify(ctx, cli, s.Namespace, s.Name)
	tsa.Verify(ctx, cli, s.Namespace, s.Name)
	rekor.Verify(ctx, cli, s.Namespace, s.Name, dbPresent)
	ctlog.Verify(ctx, cli, s.Namespace, s.Name)
	tuf.Verify(ctx, cli, s.Namespace, s.Name)
	securesign.Verify(ctx, cli, s.Namespace, s.Name)
}

func VerifyByCosignCustom(ctx context.Context, cli runtimeCli.Client,
	fulcio *rhtasv1alpha1.Fulcio,
	rekor *rhtasv1alpha1.Rekor,
	tuf *rhtasv1alpha1.Tuf,
	timestampAuthority *rhtasv1alpha1.TimestampAuthority,
	targetImageName string) {

	var m *filemutex.FileMutex
	var err error
	lockFilePath := filepath.Join(os.TempDir(), lockFileName)
	m, err = filemutex.New(lockFilePath)
	if err != nil {
		panic(err)
	}
	_ = m.Lock()
	defer func() { _ = m.Unlock() }()

	Eventually(func() error {
		return tsa.GetCertificateChain(ctx, cli, timestampAuthority.Namespace, timestampAuthority.Name, timestampAuthority.Status.Url)
	}).Should(Succeed())

	oidcToken, err := support.OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

	// sleep for a while to be sure everything has settled down
	time.Sleep(time.Duration(10) * time.Second)

	Expect(clients.Execute("cosign", "initialize", "--mirror="+tuf.Status.Url, "--root="+tuf.Status.Url+"/root.json")).To(Succeed())

	Expect(clients.Execute(
		"cosign", "sign", "-y",
		"--fulcio-url="+fulcio.Status.Url,
		"--rekor-url="+rekor.Status.Url,
		"--timestamp-server-url="+timestampAuthority.Status.Url+"/api/v1/timestamp",
		"--oidc-issuer="+support.OidcIssuerUrl(),
		"--oidc-client-id="+support.OidcClientID(),
		"--identity-token="+oidcToken,
		targetImageName,
	)).To(Succeed())

	Expect(clients.Execute(
		"cosign", "verify",
		"--rekor-url="+rekor.Status.Url,
		"--timestamp-certificate-chain=ts_chain.pem",
		"--certificate-identity-regexp", ".*@redhat",
		"--certificate-oidc-issuer-regexp", ".*keycloak.*",
		targetImageName,
	)).To(Succeed())
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

	VerifyByCosignCustom(ctx, cli, f, r, t, ts, targetImageName)
}
