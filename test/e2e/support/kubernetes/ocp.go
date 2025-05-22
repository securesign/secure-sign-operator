package kubernetes

import (
	"strings"

	"github.com/securesign/operator/internal/config"
	"k8s.io/client-go/discovery"
	cconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
)

var isOpenshift = false

func init() {
	c := cconfig.GetConfigOrDie()
	client, err := discovery.NewDiscoveryClientForConfig(c)
	if err != nil {
		panic(err)
	}
	apiGroups, err := client.ServerGroups()
	if err != nil {
		panic(err)
	}
	for _, group := range apiGroups.Groups {
		if strings.Contains(group.Name, "openshift.io") {
			isOpenshift = true
			config.Openshift = true
		}
	}
}

func IsRemoteClusterOpenshift() bool {
	return isOpenshift
}
