package tas

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

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

func CosignSign(ctx context.Context, targetImageName, tufUrl, fulcioUrl, rekorUrl, tsaUrl string) {
	ensureCosignConfig()
	oidcToken, err := support.OidcToken(ctx)
	Expect(err).ToNot(HaveOccurred())
	Expect(oidcToken).ToNot(BeEmpty())

	signArgs := []string{"sign", "-y", "--identity-token=" + oidcToken, targetImageName}
	if !useSigningConfig {
		signArgs = append(signArgs, "--fulcio-url="+fulcioUrl,
			"--rekor-url="+rekorUrl,
			"--timestamp-server-url="+tsaUrl+tsaActions.TimestampPath,
			"--oidc-issuer="+support.OidcIssuerUrl(),
			"--oidc-client-id="+support.OidcClientID())
	}
	Expect(clients.Execute("cosign", signArgs...)).To(Succeed())
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
	Eventually(func() error {
		resp, err := http.Get(tufUrl + "/root.json")
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("TUF root.json not ready: status %d", resp.StatusCode)
		}
		return nil
	}).Should(Succeed())
	Expect(clients.Execute("cosign", "initialize", "--mirror="+tufUrl, "--root="+tufUrl+"/root.json")).To(Succeed())
	CosignSign(ctx, targetImageName, tufUrl, fulcioUrl, rekorUrl, tsaUrl)
	CosignVerify(ctx, targetImageName, rekorUrl, tsaUrl)
}
