package apis

import v1 "github.com/securesign/operator/api/v1"

type ServiceReferencer interface {
	GetServiceRef() v1.ServiceReference
}
