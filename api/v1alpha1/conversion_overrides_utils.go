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

const schemeSeparator = "//"

// buildURL builds a URL from an address, optional port, and optional path.
// When address is empty the result is a schemeless authority-form URI per
// RFC 3986 (e.g. "//:8080/prefix" or "///prefix").
func buildURL(address string, port *int32, path string) (string, error) {
	if !strings.Contains(address, schemeSeparator) {
		address = schemeSeparator + address
	}

	u, err := url.Parse(address)
	if err != nil {
		return "", err
	}
	if port != nil {
		u.Host = net.JoinHostPort(u.Hostname(), strconv.FormatInt(int64(*port), 10))
	}
	if path != "" {
		u.Path = u.JoinPath(path).Path
		if !strings.HasPrefix(u.Path, "/") {
			u.Path = "/" + u.Path
		}
	}
	result := u.String()
	// url.String() omits "//" when Host is empty; restore for RFC 3986.
	if u.Scheme == "" && u.Host == "" && result != "" && !strings.HasPrefix(result, schemeSeparator) {
		result = schemeSeparator + result
	}
	return result, nil
}

// splitURLPath splits rawURL into path and everything else. Query/fragment stay on
// base so a later buildURL doesn't lose them.
func splitURLPath(rawURL string) (base, path string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	path = strings.TrimLeft(u.Path, "/")
	u.Path = ""
	u.RawPath = ""
	return u.String(), path, nil
}

// serviceReferenceToAddressPort splits a URL into address (everything but the port) and port.
func serviceReferenceToAddressPort(in *v1.ServiceReference, address *string, port **int32) error {
	if in.URL == "" {
		return nil
	}
	u, err := url.Parse(in.URL)
	if err != nil {
		return err
	}
	if u.Port() != "" {
		p, err := strconv.ParseInt(u.Port(), 10, 32)
		if err != nil {
			return fmt.Errorf("parsing port in URL %q: %w", in.URL, err)
		}
		p32 := int32(p)
		*port = &p32
	}
	host := u.Hostname()
	if host == "" {
		// no host: scheme/path/query alone aren't a meaningful address
		return nil
	}
	// re-bracket IPv6: Hostname() strips brackets, needed once port is split out
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	*address = u.String()
	return nil
}

// grpcTargetPortRe matches a trailing :port. Not net/url-based: a bare "host:port"
// target has no scheme, and url.Parse misreads that as scheme "host".
var grpcTargetPortRe = regexp.MustCompile(`:(\d+)$`)

// grpcServiceReferenceToAddressPort splits a gRPC target (grpc/grpc's doc/naming.md)
// into address and port.
func grpcServiceReferenceToAddressPort(in *v1.ServiceReference, address *string, port **int32) {
	if in.URL == "" {
		return
	}
	m := grpcTargetPortRe.FindStringSubmatchIndex(in.URL)
	if m == nil {
		*address = in.URL
		return
	}
	p, err := strconv.ParseInt(in.URL[m[2]:m[3]], 10, 32)
	if err != nil {
		*address = in.URL
		return
	}
	p32 := int32(p)
	*port = &p32
	*address = in.URL[:m[0]]
}
