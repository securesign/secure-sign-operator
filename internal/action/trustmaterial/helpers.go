package trustmaterial

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"

	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	httputils "github.com/securesign/operator/internal/utils/http"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// hasRefreshAcknowledgement reports whether the user has annotated instance to
// accept a detected trust material change.
func hasRefreshAcknowledgement(instance client.Object) bool {
	v, ok := instance.GetAnnotations()[annotations.RefreshTrustMaterial]
	if !ok {
		return false
	}
	b, _ := strconv.ParseBool(v)
	return b
}

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

// FetchPEMOverHTTP fetches raw bytes from fullURL, loading instance's
// configured TrustedCA bundle first.
func FetchPEMOverHTTP(ctx context.Context, cli client.Client, instance interface {
	client.Object
	apis.TlsClient
}, fullURL string) ([]byte, error) {
	cas, err := httputils.LoadTrustedCAs(ctx, cli, instance)
	if err != nil {
		return nil, err
	}
	return httputils.FetchFromAPI(ctx, httputils.GetClientBuilder()(cas...), fullURL)
}

// FindReadyInstance lists all objects of list's concrete type in namespace
// and returns the first one whose Ready condition is True, or
// [ErrNoReadyInstance] if none are.
func FindReadyInstance(ctx context.Context, cli client.Client, namespace string, list client.ObjectList) (apis.ConditionsAwareObject, error) {
	if err := cli.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("listing component instances: %w", err)
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil, reconcile.TerminalError(err)
	}
	for _, item := range items {
		condAware, ok := item.(apis.ConditionsAwareObject)
		if !ok {
			continue
		}
		if meta.IsStatusConditionTrue(condAware.GetConditions(), constants.ReadyCondition) {
			return condAware, nil
		}
	}
	return nil, ErrNoReadyInstance
}

// ValidatePEM checks that data contains at least one PEM block and that every
// block parses as either a certificate or a public key.
func ValidatePEM(data []byte) error {
	rest := data
	blocks := 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		blocks++
		if _, err := x509.ParseCertificate(block.Bytes); err == nil {
			continue
		}
		if _, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
			continue
		}
		return fmt.Errorf("%w: PEM block is neither a valid certificate nor a public key", ErrInvalidPEM)
	}
	if blocks == 0 {
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
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidPEM, err)
	}
	return pem.EncodeToMemory(block), nil
}
