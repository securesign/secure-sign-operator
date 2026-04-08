package apis

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// TasService represents a tas service that can be used to get the address, port and suffix.
type TasService interface {
	GetAddress() string
	GetPort() *int32

	SetAddress(address string)
	SetPort(port *int32)
}

func ServiceAsUrl(service TasService) (string, error) {
	address := service.GetAddress()

	// The authority component is preceded by a double slash ("//") and is
	// terminated by the next slash ("/"), question mark ("?"), or number
	// sign ("#") character, or by the end of the URI.
	// https://datatracker.ietf.org/doc/html/rfc3986#autoid-19
	if !strings.Contains(address, "//") {
		// fix the authority address
		address = fmt.Sprintf("//%s", address)
	}
	u, err := url.Parse(address)
	if err != nil {
		return "", err
	}
	if service.GetPort() != nil {
		u.Host = net.JoinHostPort(u.Hostname(), strconv.FormatInt(int64(*service.GetPort()), 10))
	}

	return strings.TrimPrefix(u.String(), "//"), nil
}
