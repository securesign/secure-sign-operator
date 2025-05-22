package kubernetes

import (
	"strings"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

func IsRemoteClusterOpenshift(config *rest.Config) (bool, error) {
	client, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return false, err
	}
	apiGroups, err := client.ServerGroups()
	if err != nil {
		return false, err
	}
	for _, group := range apiGroups.Groups {
		if strings.Contains(group.Name, "openshift.io") {
			return true, nil
		}
	}
	return false, nil
}
