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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/csaupgrade"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
)

const (
	FieldManager             = "securesign-operator"
	inContainerNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	kubeConfigEnvVar         = "KUBECONFIG"

	// LegacyFieldManager is the field manager the pre-SSA reconciler
	// (controllerutil.CreateOrUpdate, an Update operation) left on objects
	// already running on clusters. The API server derived it from the binary's
	// User-Agent, i.e. the operator binary name "manager" (see the "-o manager"
	// build and ENTRYPOINT ["/manager"] in the Dockerfiles). Its lingering
	// Update ownership co-owns the fields we manage, which blocks SSA from
	// pruning fields we stop declaring.
	LegacyFieldManager = "manager"
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
	return fmt.Sprintf(config.IngressHostTemplate, svcName, ns), nil
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

func Create[T client.Object](ctx context.Context, cli client.Client, obj T, fn ...func(object T) error) error {
	var err error
	for _, f := range fn {
		err = errors.Join(err, f(obj))
	}
	if err != nil {
		return err
	}
	return cli.Create(ctx, obj)
}

// ControllerOwnerRef builds an OwnerReferenceApplyConfiguration suitable for
// typed SSA apply configs. It mirrors controllerutil.SetControllerReference but
// returns an apply configuration fragment instead of mutating a live object.
func ControllerOwnerRef(owner client.Object, scheme *runtime.Scheme) (*metav1ac.OwnerReferenceApplyConfiguration, error) {
	gvk, err := apiutil.GVKForObject(owner, scheme)
	if err != nil {
		return nil, err
	}
	return metav1ac.OwnerReference().
		WithAPIVersion(gvk.GroupVersion().String()).
		WithKind(gvk.Kind).
		WithName(owner.GetName()).
		WithUID(owner.GetUID()).
		WithController(true).
		WithBlockOwnerDeletion(true), nil
}

// migrateLegacyManagedFields converts the stale LegacyFieldManager Update-operation
// ownership on an existing object into our Apply manager (FieldManager), so that a
// subsequent server-side apply can prune fields the operator no longer declares.
// It is idempotent: csaupgrade returns an empty patch once there is nothing left to
// convert, making this a cheap no-op on every later reconcile.
func migrateLegacyManagedFields(ctx context.Context, cli client.Client, live client.Object) error {
	patch, err := csaupgrade.UpgradeManagedFieldsPatch(live, sets.New(LegacyFieldManager), FieldManager)
	if err != nil {
		return fmt.Errorf("computing managed-fields upgrade patch: %w", err)
	}
	if patch == nil {
		return nil
	}
	if err := cli.Patch(ctx, live, client.RawPatch(types.JSONPatchType, patch)); err != nil {
		return fmt.Errorf("applying managed-fields upgrade patch: %w", err)
	}
	return nil
}

// Apply performs a Server-Side Apply for named, idempotent resources using typed
// apply configurations. It checks the pause annotation, migrates legacy field
// ownership, and returns true when the object was created or changed.
//
// For one-shot GenerateName resources (Jobs, config Secrets) use Create instead.
func Apply(ctx context.Context, cli client.Client, obj runtime.ApplyConfiguration, live client.Object) (bool, error) {
	key := client.ObjectKeyFromObject(live)

	resourceVersion := ""
	if err := cli.Get(ctx, key, live); err != nil {
		if !apiErrors.IsNotFound(err) {
			return false, err
		}
	} else {
		annoStr, found := live.GetAnnotations()[annotations.PausedReconciliation]
		if found {
			if paused, _ := strconv.ParseBool(annoStr); paused {
				return false, nil
			}
		}
		if err := migrateLegacyManagedFields(ctx, cli, live); err != nil {
			return false, err
		}
		resourceVersion = live.GetResourceVersion()
	}

	if err := cli.Apply(ctx, obj, client.FieldOwner(FieldManager), client.ForceOwnership); err != nil {
		return false, err
	}

	// Re-read to get the post-apply resourceVersion for change detection.
	if err := cli.Get(ctx, key, live); err != nil {
		return false, err
	}
	return live.GetResourceVersion() != resourceVersion, nil
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
