// Package fuzzer provides random generators for network-address and URL-shaped
// fuzz-test inputs: hostnames, IPv4/IPv6 literals, ports, and paths.
package fuzzer

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/randfill"
)

var pathSegmentNames = []string{"path", "sub", "api", "v1", "v2", "resource", "item", "config"}

// URLPath returns a random 1-3 segment path, sometimes with a trailing slash.
func URLPath(c randfill.Continue) string {
	n := c.Intn(3) + 1
	segments := make([]string, n)
	for i := range segments {
		segments[i] = fmt.Sprintf("%s-%d", pathSegmentNames[c.Intn(len(pathSegmentNames))], c.Intn(100))
	}
	p := strings.Join(segments, "/")
	if c.Bool() {
		p += "/"
	}
	return p
}

// Host returns a random DNS name, IPv4, or IPv6 literal — unbracketed even for IPv6,
// so bracket it yourself (bracketHost) before using it directly in a string.
func Host(c randfill.Continue) string {
	switch c.Intn(4) {
	case 0:
		return fmt.Sprintf("svc-%d.ns.svc", c.Intn(1000))
	case 1:
		return fmt.Sprintf("host-%d.example.org", c.Intn(1000))
	case 2:
		ip := make(net.IP, net.IPv4len)
		_, _ = c.Read(ip)
		return ip.String()
	default:
		ip := make(net.IP, net.IPv6len)
		_, _ = c.Read(ip)
		return ip.String()
	}
}

// randPort returns a random valid port number.
func randPort(c randfill.Continue) int {
	return c.Intn(65534) + 1
}

// Port returns a random valid port number, or nil to represent an absent one.
func Port(c randfill.Continue) *int32 {
	if c.Bool() {
		return ptr.To(int32(randPort(c)))
	}
	return nil
}

// bracketHost brackets an IPv6 literal (RFC 3986), so its colons aren't read as a
// port separator. No-op for anything else.
func bracketHost(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// hostPort returns a host, with a random port if withPort.
func hostPort(c randfill.Continue, withPort bool) string {
	host := Host(c)
	if withPort {
		return net.JoinHostPort(host, fmt.Sprintf("%d", randPort(c)))
	}
	return bracketHost(host)
}

// HTTPURL returns a random http(s) URL, or empty. withPort/withPath force that
// piece on/off; pass withPath=false for a bare base to append a path onto later.
func HTTPURL(c randfill.Continue, withPort, withPath bool) string {
	if c.Intn(4) == 0 {
		return ""
	}
	u := url.URL{Scheme: "http"}
	if c.Bool() {
		u.Scheme = "https"
	}
	if c.Bool() {
		u.User = url.UserPassword(fmt.Sprintf("user-%d", c.Intn(100)), fmt.Sprintf("pass-%d", c.Intn(100)))
	}
	u.Host = hostPort(c, withPort)
	if withPath {
		// always adds path and/or query — never a no-op
		switch c.Intn(3) {
		case 0:
			u.Path = "/" + URLPath(c)
		case 1:
			u.RawQuery = fmt.Sprintf("token=%d", c.Intn(1000000))
		default:
			u.Path = "/" + URLPath(c)
			u.RawQuery = fmt.Sprintf("token=%d", c.Intn(1000000))
		}
	}
	return u.String()
}

// GRPCURL returns a random "dns:///host[:port]" gRPC target, or empty.
func GRPCURL(c randfill.Continue, withPort bool) string {
	if c.Intn(4) == 0 {
		return ""
	}
	authority := ""
	if c.Bool() {
		authority = hostPort(c, c.Bool())
	}
	return "dns://" + authority + "/" + hostPort(c, withPort)
}
