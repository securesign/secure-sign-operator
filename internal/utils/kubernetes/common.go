package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/config"
	cLabels "github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	k8sLabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v13 "github.com/openshift/api/operator/v1"
	corev1 "k8s.io/api/core/v1"
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

// GetOpenshiftPodSecurityContextRestricted return the PodSecurityContext (https://docs.openshift.com/container-platform/4.12/authentication/managing-security-context-constraints.html):
// FsGroup set to the minimum value in the "openshift.io/sa.scc.supplemental-groups" annotation if exists, else falls back to minimum value "openshift.io/sa.scc.uid-range" annotation.
func GetOpenshiftPodSecurityContextRestricted(ctx context.Context, client client.Client, namespace string) (*corev1.PodSecurityContext, error) {
	ns := &corev1.Namespace{}
	err := client.Get(ctx, types.NamespacedName{Name: namespace}, ns)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace %q: %w", namespace, err)
	}

	uidRange, ok := ns.ObjectMeta.Annotations["openshift.io/sa.scc.uid-range"]
	if !ok {
		return nil, errors.New("annotation 'openshift.io/sa.scc.uid-range' not found")
	}

	supplementalGroups, ok := ns.ObjectMeta.Annotations["openshift.io/sa.scc.supplemental-groups"]
	if !ok {
		supplementalGroups = uidRange
	}

	supplementalGroups = strings.Split(supplementalGroups, ",")[0]
	fsGroupStr := strings.Split(supplementalGroups, "/")[0]
	fsGroup, err := strconv.ParseInt(fsGroupStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to convert fsgroup to integer %q: %w", fsGroupStr, err)
	}

	psc := corev1.PodSecurityContext{
		FSGroup:             &fsGroup,
		FSGroupChangePolicy: utils.Pointer(corev1.FSGroupChangeOnRootMismatch),
	}

	return &psc, nil
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
