package cosign

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"
)

var (
	useSigningConfig bool
	cosignInitOnce   sync.Once
)

type LocalCosign struct {
	fulcioUrl string
	rekorUrl  string
	tsaUrl    string
	tufUrl    string
}

func NewLocalCosign(tufUrl, fulcioUrl, rekorUrl, tsaUrl string) *LocalCosign {
	return &LocalCosign{
		fulcioUrl: fulcioUrl,
		rekorUrl:  rekorUrl,
		tsaUrl:    tsaUrl,
		tufUrl:    tufUrl,
	}
}

func (c *LocalCosign) Sign(ctx context.Context, targetImageName string) error {
	ensureCosignConfig()
	oidcToken, err := support.OidcToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get OIDC token: %w", err)
	}
	if oidcToken == "" {
		return fmt.Errorf("received empty OIDC token")
	}

	signArgs := []string{"sign", "-y", "--identity-token=" + oidcToken, targetImageName}
	if !useSigningConfig {
		signArgs = append(signArgs, "--fulcio-url="+c.fulcioUrl,
			"--rekor-url="+c.rekorUrl,
			"--timestamp-server-url="+c.tsaUrl,
			"--oidc-issuer="+support.OidcIssuerUrl(),
			"--oidc-client-id="+support.OidcClientID())
	}
	return clients.Execute("cosign", signArgs...)
}

func (c *LocalCosign) Verify(ctx context.Context, targetImageName string) error {
	ensureCosignConfig()
	verifyArgs := []string{"verify",
		"--certificate-identity-regexp", ".*@redhat",
		"--certificate-oidc-issuer-regexp", ".*keycloak.*",
		targetImageName}
	if !useSigningConfig {
		err := tsa.GetCertificateChain(ctx, c.tsaUrl)
		if err != nil {
			return fmt.Errorf("failed to get certificate chain: %w", err)
		}
		verifyArgs = append(verifyArgs, "--rekor-url="+c.rekorUrl, "--timestamp-certificate-chain=ts_chain.pem")
	}
	return clients.Execute("cosign", verifyArgs...)
}

func ensureCosignConfig() {
	cosignInitOnce.Do(func() {
		out, err := clients.ExecuteWithOutput("cosign", "help")
		Expect(err).ToNot(HaveOccurred(), "cosign check failed: is cosign installed in this environment?")
		useSigningConfig = strings.Contains(string(out), "signing-config")
	})
}

func (c *LocalCosign) VerifyByCosign(ctx context.Context, targetImageName string) {
	Eventually(func(ctx context.Context) error {
		return clients.Execute("cosign", "initialize", "--mirror="+c.tufUrl, "--root="+c.tufUrl+"/root.json")
	}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
	Eventually(func(ctx context.Context) error {
		return c.Sign(ctx, targetImageName)
	}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
	Eventually(func(ctx context.Context) error {
		return c.Verify(ctx, targetImageName)
	}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
}
