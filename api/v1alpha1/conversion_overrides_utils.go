package v1alpha1

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	v1 "github.com/securesign/operator/api/v1"
)

var portRe = regexp.MustCompile(`:(\d+)(?:/|$)`)

const schemelessPrefix = "schemeless://"

// ensureScheme prepends a placeholder scheme if rawURL has none,
// so that url.Parse treats the host:port correctly instead of
// misinterpreting the host as a scheme.
func ensureScheme(rawURL string) (string, bool) {
	if strings.Contains(rawURL, "://") {
		return rawURL, false
	}
	return schemelessPrefix + rawURL, true
}

func stripScheme(parsedURL string) string {
	return strings.TrimPrefix(parsedURL, schemelessPrefix)
}

func urlWithPath(rawUrl, path string) (string, error) {
	fixed, added := ensureScheme(rawUrl)
	u, err := url.Parse(fixed)
	if err != nil {
		return "", err
	}
	u.Path = path
	result := u.String()
	if added {
		result = stripScheme(result)
	}
	return result, nil
}

func urlWithoutPath(rawUrl string) (string, error) {
	base, _, err := splitURLPath(rawUrl)
	return base, err
}

func splitURLPath(rawURL string) (base, path string, err error) {
	fixed, added := ensureScheme(rawURL)
	u, err := url.Parse(fixed)
	if err != nil {
		return "", "", fmt.Errorf("parsing URL %q: %w", rawURL, err)
	}
	path = strings.TrimPrefix(u.Path, "/")
	u.Path = ""
	u.RawPath = ""
	base = u.String()
	if added {
		base = stripScheme(base)
	}
	return base, path, nil
}

func addressPortToServiceReference(address string, port *int32, out *v1.ServiceReference) {
	if address != "" && port != nil {
		out.URL = fmt.Sprintf("%s:%d", address, *port)
	} else if address != "" {
		out.URL = address
	}
}

func serviceReferenceToAddressPort(in *v1.ServiceReference, address *string, port **int32) {
	if in.URL == "" {
		return
	}
	m := portRe.FindStringSubmatchIndex(in.URL)
	if m == nil {
		*address = in.URL
		return
	}
	*address = in.URL[:m[0]]
	p, err := strconv.ParseInt(in.URL[m[2]:m[3]], 10, 32)
	if err != nil {
		*address = in.URL
		return
	}
	p32 := int32(p)
	*port = &p32
}
