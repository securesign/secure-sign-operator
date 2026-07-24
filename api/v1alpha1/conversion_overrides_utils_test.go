package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
	rhtasv1 "github.com/securesign/operator/api/v1"
	"k8s.io/utils/ptr"
)

func TestAddressPortToServiceReference(t *testing.T) {
	tests := []struct {
		name    string
		address string
		port    *int32
		wantURL string
	}{
		{
			name:    "address with port",
			address: "rekor.ns.svc",
			port:    ptr.To(int32(8080)),
			wantURL: "rekor.ns.svc:8080",
		},
		{
			name:    "address without port",
			address: "rekor.ns.svc",
			port:    nil,
			wantURL: "rekor.ns.svc",
		},
		{
			name:    "empty address",
			address: "",
			port:    ptr.To(int32(8080)),
			wantURL: "",
		},
		{
			name:    "both empty",
			address: "",
			port:    nil,
			wantURL: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			out := &rhtasv1.ServiceReference{}
			addressPortToServiceReference(tt.address, tt.port, out)
			g.Expect(out.URL).To(Equal(tt.wantURL))
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
			url:         "rekor.ns.svc:8080",
			wantAddress: "rekor.ns.svc",
			wantPort:    ptr.To(int32(8080)),
		},
		{
			name:        "url without port",
			url:         "rekor.ns.svc",
			wantAddress: "rekor.ns.svc",
			wantPort:    nil,
		},
		{
			name:        "empty url",
			url:         "",
			wantAddress: "",
			wantPort:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var address string
			var port *int32
			serviceReferenceToAddressPort(&rhtasv1.ServiceReference{URL: tt.url}, &address, &port)
			g.Expect(address).To(Equal(tt.wantAddress))
			g.Expect(port).To(Equal(tt.wantPort))
		})
	}
}

func TestAddressPortRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		address string
		port    *int32
	}{
		{"with port", "rekor.ns.svc", ptr.To(int32(3000))},
		{"without port", "rekor.ns.svc", nil},
		{"empty", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			ref := &rhtasv1.ServiceReference{}
			addressPortToServiceReference(tt.address, tt.port, ref)

			var gotAddress string
			var gotPort *int32
			serviceReferenceToAddressPort(ref, &gotAddress, &gotPort)

			g.Expect(gotAddress).To(Equal(tt.address))
			g.Expect(gotPort).To(Equal(tt.port))
		})
	}
}

func TestUrlWithPath(t *testing.T) {
	g := NewWithT(t)

	got, err := urlWithPath("http://tsa.ns.svc:3000/old", "/api/v1/timestamp")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(Equal("http://tsa.ns.svc:3000/api/v1/timestamp"))

	got, err = urlWithPath("http://tsa.ns.svc:3000", "/api/v1/timestamp")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(Equal("http://tsa.ns.svc:3000/api/v1/timestamp"))
}

func TestUrlWithoutPath(t *testing.T) {
	g := NewWithT(t)

	got, err := urlWithoutPath("http://tsa.ns.svc:3000/api/v1/timestamp")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(Equal("http://tsa.ns.svc:3000"))

	got, err = urlWithoutPath("http://tsa.ns.svc:3000")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(Equal("http://tsa.ns.svc:3000"))
}

func TestSplitURLPath(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantBase string
		wantPath string
		wantErr  bool
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
			wantPath: "",
		},
		{
			name:     "url with nested path",
			url:      "http://ctlog.ns.svc:8080/a/b/c",
			wantBase: "http://ctlog.ns.svc:8080",
			wantPath: "a/b/c",
		},
		{
			name:     "url with trailing slash",
			url:      "http://ctlog.ns.svc:8080/",
			wantBase: "http://ctlog.ns.svc:8080",
			wantPath: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			base, path, err := splitURLPath(tt.url)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(base).To(Equal(tt.wantBase))
			g.Expect(path).To(Equal(tt.wantPath))
		})
	}
}
