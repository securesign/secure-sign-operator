package http

import (
	"net/http"
)

type RoundTripFunc func(req *http.Request) *http.Response

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
