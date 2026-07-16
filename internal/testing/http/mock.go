package http

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	httputils "github.com/securesign/operator/internal/utils/http"
)

type RoundTripFunc func(req *http.Request) *http.Response

// RoundTrip implements http.RoundTripper, allowing RoundTripFunc to be used directly as a Transport.
func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

// MockTransport is an implementation of http.RoundTripper to mock HTTP responses for specific URLs.
type MockTransport struct {
	defaultTransport http.RoundTripper
	mock             map[string]RoundTripFunc
}

func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if val, ok := m.mock[req.URL.String()]; ok {
		return val(req), nil
	}

	// Pass through to the default transport for other URLs
	return m.defaultTransport.RoundTrip(req)
}

// SetMockTransport sets the custom RoundTripper as the transport for http.DefaultClient for a specific URL.
func SetMockTransport(client *http.Client, mock map[string]RoundTripFunc) {
	client.Transport = &MockTransport{
		defaultTransport: http.DefaultTransport,
		mock:             mock,
	}
}

// RestoreDefaultTransport restores the default transport for http.DefaultClient.
func RestoreDefaultTransport(client *http.Client) {
	client.Transport = http.DefaultTransport
}

// StubClientBuilder points httputils' shared HTTP client builder at a client
// that returns status/body for every request to url, restoring the previous
// builder via t.Cleanup.
func StubClientBuilder(t testing.TB, url string, status int, body string) {
	mockClient := &http.Client{}
	SetMockTransport(mockClient, map[string]RoundTripFunc{
		url: func(_ *http.Request) *http.Response {
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(bytes.NewReader([]byte(body))),
				Header:     make(http.Header),
			}
		},
	})
	orig := httputils.GetClientBuilder()
	httputils.SetClientBuilder(func(_ ...[]byte) *http.Client { return mockClient })
	t.Cleanup(func() { httputils.SetClientBuilder(orig) })
}
