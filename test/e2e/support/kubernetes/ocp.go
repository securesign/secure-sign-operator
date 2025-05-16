package kubernetes

import (
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

func IsRemoteClusterOpenshift(config *rest.Config) (bool, error) {
	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false, err
	}
	apiGroups, err := client.ServerGroups()

	for _, group := range apiGroups.Groups {
		if group.Name == "route.openshift.io" || group.Name == "config.openshift.io" {
			return true, nil
		}
	}
	return false, nil
}
