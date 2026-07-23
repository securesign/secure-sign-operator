package v1alpha1

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	v1 "github.com/securesign/operator/api/v1"
)

// parseURL parses rawURL and validates that scheme and host are present.
func parseURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL %q: %w", rawURL, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("URL %q is missing scheme", rawURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL %q is missing host", rawURL)
	}
	return u, nil
}

// buildURL builds a URL from an address, optional port, and optional path.
func buildURL(address string, port *int32, path string) (string, error) {
	if address == "" {
		return "", nil
	}

	u, err := parseURL(address)
	if err != nil {
		return "", err
	}
	if port != nil {
		u.Host = net.JoinHostPort(u.Hostname(), strconv.FormatInt(int64(*port), 10))
	}
	if path != "" {
		u.Path = path
	}
	return u.String(), nil
}

// splitURLPath splits rawURL into its origin (scheme://host[:port]) and path components.
func splitURLPath(rawURL string) (base, path string, err error) {
	u, err := parseURL(rawURL)
	if err != nil {
		return "", "", err
	}
	path = strings.TrimPrefix(u.Path, "/")
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), path, nil
}

// serviceReferenceToAddressPort decomposes an URL into scheme://host and port.
func serviceReferenceToAddressPort(in *v1.ServiceReference, address *string, port **int32) error {
	if in.URL == "" {
		return nil
	}
	u, err := parseURL(in.URL)
	if err != nil {
		return err
	}
	h, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		*address = u.Scheme + "://" + u.Host
		return nil
	}
	*address = u.Scheme + "://" + h
	p, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		return fmt.Errorf("parsing port in URL %q: %w", in.URL, err)
	}
	p32 := int32(p)
	*port = &p32
	return nil
}

// grpcServiceReferenceToAddressPort decomposes a gRPC URI (e.g. dns:///host:port)
// by stripping the last :port match via regex. gRPC uses a special name resolution
// scheme: https://github.com/grpc/grpc/blob/master/doc/naming.md
func grpcServiceReferenceToAddressPort(in *v1.ServiceReference, address *string, port **int32) {
	if in.URL == "" {
		return
	}
	portRe := regexp.MustCompile(`:(\d+)(?:/|$)`)
	matches := portRe.FindAllStringSubmatchIndex(in.URL, -1)
	if len(matches) == 0 {
		*address = in.URL
		return
	}
	m := matches[len(matches)-1]
	*address = in.URL[:m[0]]
	p, err := strconv.ParseInt(in.URL[m[2]:m[3]], 10, 32)
	if err != nil {
		*address = in.URL
		return
	}
	p32 := int32(p)
	*port = &p32
}
