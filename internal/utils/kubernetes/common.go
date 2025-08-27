package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	cLabels "github.com/securesign/operator/internal/labels"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	k8sLabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	inContainerNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	kubeConfigEnvVar         = "KUBECONFIG"
)

func FilterOutCommonLabels(labels map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range labels {
		switch key {
		case cLabels.LabelAppPartOf, cLabels.LabelAppInstance, cLabels.LabelAppComponent, cLabels.LabelAppManagedBy, cLabels.LabelAppName:
		default:
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
	return config.Openshift
}

func CalculateHostname(ctx context.Context, client client.Client, svcName, ns string) (string, error) {
	if IsOpenShift() {
		ingress := &configv1.Ingress{}
		if err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, ingress); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s-%s.%s", svcName, ns, ingress.Spec.Domain), nil
	}
	return svcName + ".local", nil
}

func FindByLabelSelector(ctx context.Context, c client.Client, list client.ObjectList, namespace, labelSelector string) error {
	selector, err := k8sLabels.Parse(labelSelector)
	listOptions := &client.ListOptions{
		LabelSelector: selector,
	}
	if err != nil {
		return err
	}

	return c.List(ctx, list, client.InNamespace(namespace), listOptions)
}

func CreateOrUpdate[T client.Object](ctx context.Context, cli client.Client, obj T, fn ...func(object T) error) (result controllerutil.OperationResult, err error) {
	err = retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apiErrors.IsConflict(err) || apiErrors.IsAlreadyExists(err)
	}, func() error {
		var createUpdateError error
		result, createUpdateError = controllerutil.CreateOrUpdate(ctx, cli, obj, func() (fnError error) {
			annoStr, find := obj.GetAnnotations()[annotations.PausedReconciliation]
			if find {
				annoBool, _ := strconv.ParseBool(annoStr)
				if annoBool {
					return
				}
			}
			for _, f := range fn {
				fnError = errors.Join(fnError, f(obj))
			}
			return
		})
		return createUpdateError
	})
	return
}
