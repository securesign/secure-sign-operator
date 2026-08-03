package v1alpha1

import (
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestBuildURL(t *testing.T) {
	tests := []struct {
		name    string
		address string
		port    *int32
		path    string
		wantURL string
	}{
		{
			name:    "address with scheme and port",
			address: "http://rekor.ns.svc",
			port:    ptr.To(int32(8080)),
			wantURL: "http://rekor.ns.svc:8080",
		},
		{
			name:    "address with scheme without port",
			address: "http://rekor.ns.svc",
			wantURL: "http://rekor.ns.svc",
		},
		{
			name:    "address without scheme",
			address: "rekor.ns.svc",
			port:    ptr.To(int32(8080)),
			wantURL: "//rekor.ns.svc:8080",
		},
		{
			name:    "address with port and path",
			address: "http://ctlog.ns.svc",
			port:    ptr.To(int32(6963)),
			path:    "trusted-artifact-signer",
			wantURL: "http://ctlog.ns.svc:6963/trusted-artifact-signer",
		},
		{
			name:    "address already carrying its own path, plus a prefix",
			address: "address.org/path",
			port:    ptr.To(int32(9999)),
			path:    "shard1",
			wantURL: "//address.org:9999/path/shard1",
		},
		{
			name:    "prefix itself has a trailing slash",
			address: "address.org/path",
			port:    ptr.To(int32(9999)),
			path:    "shard1/",
			wantURL: "//address.org:9999/path/shard1/",
		},
		{
			name:    "empty address with port",
			address: "",
			port:    ptr.To(int32(8080)),
			wantURL: "//:8080",
		},
		{
			name:    "empty address with port and path",
			address: "",
			port:    ptr.To(int32(6962)),
			path:    "trusted-artifact-signer",
			wantURL: "//:6962/trusted-artifact-signer",
		},
		{
			name:    "empty address with path only",
			address: "",
			path:    "trusted-artifact-signer",
			wantURL: "///trusted-artifact-signer",
		},
		{
			name:    "both empty",
			address: "",
			wantURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := buildURL(tt.address, tt.port, tt.path)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.wantURL))
		})
	}
}

func TestServiceReferenceToAddressPort(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantAddress string
		wantPort    *int32
	}{
		{
			name:        "url with port",
			url:         "http://rekor.ns.svc:8080",
			wantAddress: "http://rekor.ns.svc",
			wantPort:    ptr.To(int32(8080)),
		},
		{
			name:        "url without port",
			url:         "http://rekor.ns.svc",
			wantAddress: "http://rekor.ns.svc",
		},
		{
			name:        "empty url",
			url:         "",
			wantAddress: "",
		},
		{
			name:        "schemeless with port and path",
			url:         "//:330/path",
			wantAddress: "",
			wantPort:    ptr.To(int32(330)),
		},
		{
			name:        "schemeless with host and port",
			url:         "//host:330/path",
			wantAddress: "//host/path",
			wantPort:    ptr.To(int32(330)),
		},
		{
			name:        "url with userinfo and query",
			url:         "https://user:pass@rekor.ns.svc:8080/path?token=abc123",
			wantAddress: "https://user:pass@rekor.ns.svc/path?token=abc123",
			wantPort:    ptr.To(int32(8080)),
		},
		{
			name:        "http with empty host and port",
			url:         "http://:330/path",
			wantAddress: "",
			wantPort:    ptr.To(int32(330)),
		},
		{
			name:        "bare path only",
			url:         "user",
			wantAddress: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var address string
			var port *int32
			err := serviceReferenceToAddressPort(&rhtasv1.ServiceReference{URL: tt.url}, &address, &port)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(address).To(Equal(tt.wantAddress))
			g.Expect(port).To(Equal(tt.wantPort))
		})
	}
}

func TestBuildURLRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		address string
		port    *int32
	}{
		{"with port", "http://rekor.ns.svc", ptr.To(int32(3000))},
		{"without port", "http://rekor.ns.svc", nil},
		{"empty address with port", "", ptr.To(int32(8080))},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			url, err := buildURL(tt.address, tt.port, "")
			g.Expect(err).ToNot(HaveOccurred())

			var gotAddress string
			var gotPort *int32
			err = serviceReferenceToAddressPort(&rhtasv1.ServiceReference{URL: url}, &gotAddress, &gotPort)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(gotAddress).To(Equal(tt.address))
			g.Expect(gotPort).To(Equal(tt.port))
		})
	}
}

func TestGrpcServiceReferenceToAddressPort(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantAddress string
		wantPort    *int32
	}{
		{
			name:        "dns with default authority",
			url:         "dns:///trillian-logserver.ns.svc:8091",
			wantAddress: "dns:///trillian-logserver.ns.svc",
			wantPort:    ptr.To(int32(8091)),
		},
		{
			name:        "dns with explicit authority",
			url:         "dns://authority:53/trillian-logserver.ns.svc:8091",
			wantAddress: "dns://authority:53/trillian-logserver.ns.svc",
			wantPort:    ptr.To(int32(8091)),
		},
		{
			name:        "dns with authority port but portless target",
			url:         "dns://authority:53/trillian-logserver.ns.svc",
			wantAddress: "dns://authority:53/trillian-logserver.ns.svc",
		},
		{
			name:        "dns without port",
			url:         "dns:///trillian-logserver.ns.svc",
			wantAddress: "dns:///trillian-logserver.ns.svc",
		},
		{
			name:        "empty url",
			url:         "",
			wantAddress: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var address string
			var port *int32
			grpcServiceReferenceToAddressPort(&rhtasv1.ServiceReference{URL: tt.url}, &address, &port)
			g.Expect(address).To(Equal(tt.wantAddress))
			g.Expect(port).To(Equal(tt.wantPort))
		})
	}
}

func TestGrpcAddressPortRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		address string
		port    *int32
	}{
		{"dns with port", "dns:///trillian.ns.svc", ptr.To(int32(8091))},
		{"dns with authority and port", "dns://authority:53/trillian.ns.svc", ptr.To(int32(8091))},
		{"dns without port", "dns:///trillian.ns.svc", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ref := &rhtasv1.ServiceReference{}
			if tt.port != nil {
				ref.URL = fmt.Sprintf("%s:%d", tt.address, *tt.port)
			} else {
				ref.URL = tt.address
			}

			var gotAddress string
			var gotPort *int32
			grpcServiceReferenceToAddressPort(ref, &gotAddress, &gotPort)

			g.Expect(gotAddress).To(Equal(tt.address))
			g.Expect(gotPort).To(Equal(tt.port))
		})
	}
}

func TestSplitURLPath(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantBase string
		wantPath string
	}{
		{
			name:     "url with path",
			url:      "http://ctlog.ns.svc:8080/trusted-artifact-signer",
			wantBase: "http://ctlog.ns.svc:8080",
			wantPath: "trusted-artifact-signer",
		},
		{
			name:     "url without path",
			url:      "http://ctlog.ns.svc:8080",
			wantBase: "http://ctlog.ns.svc:8080",
		},
		{
			name:     "url with nested path",
			url:      "http://tsa.ns.svc:3000/api/v1/timestamp",
			wantBase: "http://tsa.ns.svc:3000",
			wantPath: "api/v1/timestamp",
		},
		{
			name:     "url with trailing slash",
			url:      "http://ctlog.ns.svc:8080/",
			wantBase: "http://ctlog.ns.svc:8080",
		},
		{
			name:     "schemeless with port and path",
			url:      "//:330/path",
			wantBase: "//:330",
			wantPath: "path",
		},
		{
			name:     "schemeless path only",
			url:      "///path",
			wantPath: "path",
		},
		{
			name:     "schemeless host port path",
			url:      "//host:330/path",
			wantBase: "//host:330",
			wantPath: "path",
		},
		{
			name:     "http empty host with port and path",
			url:      "http://:330/path",
			wantBase: "http://:330",
			wantPath: "path",
		},
		{
			name: "empty url",
			url:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			base, path, err := splitURLPath(tt.url)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(base).To(Equal(tt.wantBase))
			g.Expect(path).To(Equal(tt.wantPath))
		})
	}
}
