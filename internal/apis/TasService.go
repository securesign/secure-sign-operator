package apis

import "fmt"

// TasService represents a tas service that can be used to get the address, port and suffix.
type TasService interface {
	GetAddress() string
	GetPort() *int32

	SetAddress(address string)
	SetPort(port *int32)
}

func ServiceAsUrl(service TasService) string {
	address := service.GetAddress()
	if service.GetPort() != nil {
		address = fmt.Sprintf("%s:%d", service.GetAddress(), *service.GetPort())
	}
	return address
}
