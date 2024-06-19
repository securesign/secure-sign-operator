package kubernetes

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"os"
	"path/filepath"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"sync"
	"time"

	v13 "github.com/openshift/api/operator/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

var onceIsOpenshift sync.Once
var isOpenshift bool

func IsOpenShift(client client.Client) bool {
	// atomic
	onceIsOpenshift.Do(func() {
		log := ctrllog.Log.WithName("IsOpenshift")
		isOpenshift = checkIsOpenshift(client, log)
		log.Info(strconv.FormatBool(isOpenshift))
	})

	return isOpenshift
}

func checkIsOpenshift(client client.Client, logger logr.Logger) bool {

	_, err := client.RESTMapper().ResourceFor(schema.GroupVersionResource{
		Group:    "security.openshift.io",
		Resource: "SecurityContextConstraints",
	})

	for i := 0; i < openshiftCheckLimit; i++ {
		if err != nil {
			if meta.IsNoMatchError(err) {
				// continue with non-ocp standard
				return false
			}

			logger.Info("failed to identify", "retry", fmt.Sprintf("%d/%d", i, openshiftCheckLimit))
			logger.V(1).Info(err.Error())
			time.Sleep(time.Duration(i) * openshiftCheckDelay)
			continue
		}
		return true
	}

	return false
}

func CalculateHostname(ctx context.Context, client client.Client, svcName, ns string) (string, error) {
	if IsOpenShift(client) {
		ctrl := &v13.IngressController{}
		if err := client.Get(ctx, types.NamespacedName{Namespace: "openshift-ingress-operator", Name: "default"}, ctrl); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s-%s.%s", svcName, ns, ctrl.Status.Domain), nil
	}
	return svcName + ".local", nil
}
