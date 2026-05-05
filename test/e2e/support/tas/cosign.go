package tas

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tsaActions "github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/test/e2e/support/tas/tsa"

	. "github.com/onsi/gomega"
	"github.com/securesign/operator/test/e2e/support"
	clients "github.com/securesign/operator/test/e2e/support/tas/cli"
)

var (
	useSigningConfig bool
	cosignInitOnce   sync.Once
)

func ensureCosignConfig() {
	cosignInitOnce.Do(func() {
		out, err := clients.ExecuteWithOutput("cosign", "help")
		Expect(err).ToNot(HaveOccurred(), "cosign check failed: is cosign installed in this environment?")
		useSigningConfig = strings.Contains(string(out), "signing-config")
	})
}

func tryCosignSign(ctx context.Context, targetImageName, fulcioUrl, rekorUrl, tsaUrl string) error {
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
		signArgs = append(signArgs, "--fulcio-url="+fulcioUrl,
			"--rekor-url="+rekorUrl,
			"--timestamp-server-url="+tsaUrl+tsaActions.TimestampPath,
			"--oidc-issuer="+support.OidcIssuerUrl(),
			"--oidc-client-id="+support.OidcClientID())
	}
	return clients.Execute("cosign", signArgs...)
}

func CosignVerify(ctx context.Context, targetImageName, rekorUrl, tsaUrl string) {
	ensureCosignConfig()
	verifyArgs := []string{"verify",
		"--certificate-identity-regexp", ".*@redhat",
		"--certificate-oidc-issuer-regexp", ".*keycloak.*",
		targetImageName}
	if !useSigningConfig {
		Eventually(func(ctx context.Context) error {
			return tsa.GetCertificateChain(ctx, tsaUrl)
		}).WithContext(ctx).Should(Succeed())
		verifyArgs = append(verifyArgs, "--rekor-url="+rekorUrl, "--timestamp-certificate-chain=ts_chain.pem")
	}
	Expect(clients.Execute("cosign", verifyArgs...)).To(Succeed())
}

func VerifyByCosign(ctx context.Context, targetImageName, tufUrl, fulcioUrl, rekorUrl, tsaUrl string) {
	Eventually(func(ctx context.Context) error {
		return clients.Execute("cosign", "initialize", "--mirror="+tufUrl, "--root="+tufUrl+"/root.json")
	}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
	Eventually(func(ctx context.Context) error {
		return tryCosignSign(ctx, targetImageName, fulcioUrl, rekorUrl, tsaUrl)
	}).WithContext(ctx).WithPolling(2 * time.Second).Should(Succeed())
	CosignVerify(ctx, targetImageName, rekorUrl, tsaUrl)
}
