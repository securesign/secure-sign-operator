package apis

import (
	"net"
	"net/url"
	"strconv"
)

// TasService represents a tas service that can be used to get the address, port and suffix.
type TasService interface {
	GetAddress() string
	GetPort() *int32

	SetAddress(address string)
	SetPort(port *int32)
}

func ServiceAsUrl(service TasService) (string, error) {
	u, err := url.Parse(service.GetAddress())
	if err != nil {
		return "", err
	}
	if service.GetPort() != nil {
		u.Host = net.JoinHostPort(u.Hostname(), strconv.FormatInt(int64(*service.GetPort()), 10))
	}
	return u.String(), nil
}
