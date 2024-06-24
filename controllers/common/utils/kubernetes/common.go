package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	v13 "github.com/openshift/api/operator/v1"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	inContainerNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	kubeConfigEnvVar         = "KUBECONFIG"

	ComponentLabel = "app.kubernetes.io/component"
	NameLabel      = "app.kubernetes.io/name"

	openshiftCheckLimit = 10
	openshiftCheckDelay = time.Second
)

func FilterCommonLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range labels {
		if key == "app.kubernetes.io/part-of" || key == "app.kubernetes.io/instance" {
			out[key] = value
		}
	}
	return out
}

func getDefaultKubeConfigFile() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, ".kube", "config"), nil
}

func ContainerMode() (bool, error) {
	// When kube config is set, container mode is not used
	if os.Getenv(kubeConfigEnvVar) != "" {
		return false, nil
	}
	// Use container mode only when the kubeConfigFile does not exist and the container namespace file is present
	configFile, err := getDefaultKubeConfigFile()
	if err != nil {
		return false, err
	}
	configFilePresent := true
	_, err = os.Stat(configFile)
	if err != nil && os.IsNotExist(err) {
		configFilePresent = false
	} else if err != nil {
		return false, err
	}
	if !configFilePresent {
		_, err := os.Stat(inContainerNamespaceFile)
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, err
	}
	return false, nil
}

func IsOpenShift() bool {
	return constants.Openshift
}

func CalculateHostname(ctx context.Context, client client.Client, svcName, ns string) (string, error) {
	if IsOpenShift() {
		ctrl := &v13.IngressController{}
		if err := client.Get(ctx, types.NamespacedName{Namespace: "openshift-ingress-operator", Name: "default"}, ctrl); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s-%s.%s", svcName, ns, ctrl.Status.Domain), nil
	}
	return svcName + ".local", nil
}
