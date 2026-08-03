package apis

import "sigs.k8s.io/controller-runtime/pkg/client"

type AddressableObject interface {
	GetServiceURL() string
	client.Object
}

type AddressableConditionAware interface {
	AddressableObject
	ConditionsAwareObject
}
