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
func buildURL(address string, port *int32, path string) (string, error) {
	if !strings.Contains(address, schemeSeparator) {
		// use schemeless URI syntax
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
		u.Path = path
	}
	return u.String(), nil
}

// splitURLPath splits rawURL into its origin (scheme://host[:port]) and path components.
func splitURLPath(rawURL string) (base, path string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	path = strings.TrimLeft(u.Path, "/")
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
	u, err := url.Parse(in.URL)
	if err != nil {
		return err
	}
	// scheme without host is meaningless
	if u.Scheme != "" && u.Hostname() != "" {
		*address = u.Scheme + "://"
	}
	*address += u.Hostname()

	if u.Port() != "" {
		p, err := strconv.ParseInt(u.Port(), 10, 32)
		if err != nil {
			return fmt.Errorf("parsing port in URL %q: %w", in.URL, err)
		}
		p32 := int32(p)
		*port = &p32
	}

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
