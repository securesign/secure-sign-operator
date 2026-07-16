package http

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/securesign/operator/internal/apis"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var serviceCAFiles = []string{
	"/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt",
	"/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
}

var (
	builderMu sync.RWMutex
	builder   = DefaultClientBuilder
)

// GetClientBuilder returns the current HTTP client builder function.
func GetClientBuilder() func(...[]byte) *http.Client {
	builderMu.RLock()
	defer builderMu.RUnlock()
	return builder
}

// SetClientBuilder replaces the HTTP client builder (used by tests).
func SetClientBuilder(fn func(...[]byte) *http.Client) {
	builderMu.Lock()
	defer builderMu.Unlock()
	builder = fn
}

// ResetClientBuilder restores the default HTTP client builder.
func ResetClientBuilder() {
	builderMu.Lock()
	defer builderMu.Unlock()
	builder = DefaultClientBuilder
}

// DefaultClientBuilder builds a TLS-aware HTTP client that trusts the system CA pool,
// the OpenShift/Kubernetes service CA (if present on disk), and any additional PEM-encoded CA bundles.
// CA files are read on each call to pick up rotations (kubelet updates mounted files in-place).
func DefaultClientBuilder(additionalCAs ...[]byte) *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	for _, path := range serviceCAFiles {
		if data, err := os.ReadFile(path); err == nil {
			pool.AppendCertsFromPEM(data)
		}
	}
	for _, ca := range additionalCAs {
		pool.AppendCertsFromPEM(ca)
	}

	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}
}

// FetchFromAPI performs an HTTP GET request to the given URL and returns the response body.
// The request is bound to ctx so it is cancelled if the caller's context is cancelled (e.g. on shutdown).
func FetchFromAPI(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // URL is constructed by the caller from operator-controlled sources
	if err != nil {
		return nil, fmt.Errorf("fetch API: building request for %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch API: GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned status %d: %s", url, resp.StatusCode, string(body))
	}

	return body, nil
}

// LoadTrustedCAs reads instance's TrustedCA ConfigMap and returns the CA
// bundles as byte slices for use with the HTTP client builder.
func LoadTrustedCAs(ctx context.Context, cli client.Client, instance interface {
	client.Object
	apis.TlsClient
}) ([][]byte, error) {
	trustedCA := instance.GetTrustedCA()
	if trustedCA == nil {
		return nil, nil
	}
	cm := &corev1.ConfigMap{}
	if err := cli.Get(ctx, client.ObjectKey{Name: trustedCA.Name, Namespace: instance.GetNamespace()}, cm); err != nil {
		return nil, fmt.Errorf("reading TrustedCA ConfigMap %s: %w", trustedCA.Name, err)
	}
	var cas [][]byte
	for _, v := range cm.Data {
		cas = append(cas, []byte(v))
	}
	return cas, nil
}
