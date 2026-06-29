package trustmaterial

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/securesign/operator/internal/utils/kubernetes"
)

// ResolveBaseURL returns the base URL for in-cluster HTTP calls to a component's service.
// When the operator runs inside a pod (ContainerMode), it uses the internal Kubernetes DNS name
// which is always reachable. When running outside (local dev), it falls back to statusUrl.
//
// TODO: The http:// protocol is hardcoded. When internal TLS between ingress and pods is
// implemented (planned for OCP), the protocol must be resolved dynamically from the
// component's TLS configuration (e.g. service-serving-cert annotation or TLS status).
func ResolveBaseURL(deploymentName, namespace, statusUrl string, port ...int) string {
	inContainer, _ := kubernetes.ContainerMode()
	if !inContainer && statusUrl != "" {
		return statusUrl
	}
	if len(port) > 0 {
		return fmt.Sprintf("http://%s.%s.svc:%d", deploymentName, namespace, port[0])
	}
	return fmt.Sprintf("http://%s.%s.svc", deploymentName, namespace)
}

// ValidatePEM checks that data contains at least one valid PEM block.
func ValidatePEM(data []byte) error {
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("%w: no PEM block found", ErrInvalidPEM)
	}
	return nil
}

type trustBundle struct {
	Chains []certificateChain `json:"chains"`
}

type certificateChain struct {
	Certificates []string `json:"certificates"`
}

// ParseTrustBundle parses a Fulcio /api/v2/trustBundle JSON response
// and returns the concatenated PEM-encoded certificate chain.
// The chain is ordered signing-cert-first: Certs[0] is the CA certificate
// Fulcio uses to issue code-signing certificates, followed by any intermediate
// and root CAs. TUF publishes the full chain as fulcio_v1.crt.pem for client
// verification. CTlog should extract only the signing cert (first PEM block).
func ParseTrustBundle(body []byte) ([]byte, error) {
	var bundle trustBundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseTrustBundle, err)
	}

	if len(bundle.Chains) == 0 {
		return nil, fmt.Errorf("%w: no certificate chains in bundle", ErrEmptyTrustBundle)
	}

	var pemBuf strings.Builder
	for _, chain := range bundle.Chains {
		for _, cert := range chain.Certificates {
			pemBuf.WriteString(strings.TrimSpace(cert))
			pemBuf.WriteString("\n")
		}
	}

	result := strings.TrimSpace(pemBuf.String())
	if result == "" {
		return nil, fmt.Errorf("%w: chains contain no certificate data", ErrEmptyTrustBundle)
	}

	return []byte(result), nil
}

// ExtractSigningCert extracts the first PEM certificate block from concatenated
// PEM data. In a Fulcio trust bundle, the first certificate is the CA cert that
// Fulcio uses to sign code-signing certificates. CTlog uses this as its accepted
// root to control which certificates can be added to the transparency log.
func ExtractSigningCert(pemData []byte) ([]byte, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block found in trust material", ErrInvalidPEM)
	}
	return pem.EncodeToMemory(block), nil
}
