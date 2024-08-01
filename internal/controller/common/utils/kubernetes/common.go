package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/labels"

	v13 "github.com/openshift/api/operator/v1"
	"github.com/securesign/operator/internal/controller/common/utils"
	"github.com/securesign/operator/internal/controller/constants"
	corev1 "k8s.io/api/core/v1"
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
	selector, err := labels.Parse(labelSelector)
	listOptions := &client.ListOptions{
		LabelSelector: selector,
	}
	if err != nil {
		return err
	}

	return c.List(ctx, list, client.InNamespace(namespace), listOptions)
}
