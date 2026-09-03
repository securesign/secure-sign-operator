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
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/csaupgrade"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	configv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// pruning fields we stop declaring — ForceOwnership only evicts a co-owner
	// when the applied value differs, so it cannot help here. We convert that
	// ownership to our Apply manager before applying.
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
	// The patch pins resourceVersion, so a concurrent writer yields a 409.
	if err := cli.Patch(ctx, live, client.RawPatch(types.JSONPatchType, patch)); err != nil {
		return fmt.Errorf("applying managed-fields upgrade patch: %w", err)
	}
	return nil
}

func CreateOrUpdate[T client.Object](ctx context.Context, cli client.Client, obj T, fn ...func(object T) error) (controllerutil.OperationResult, error) {
	key := client.ObjectKeyFromObject(obj)

	// Read the current state only to classify the result and honor the pause
	// annotation. It is never used as the base for the applied object: SSA
	// declares the intent we own and lets the server merge, rather than
	// read-modify-writing the whole object (which would claim ownership of
	// server-set defaults and fight other field managers).
	current := obj.DeepCopyObject().(client.Object)
	exists := true
	resourceVersion := ""
	if err := cli.Get(ctx, key, current); err != nil {
		if !apiErrors.IsNotFound(err) {
			return controllerutil.OperationResultNone, err
		}
		exists = false
	} else {
		annoStr, found := current.GetAnnotations()[annotations.PausedReconciliation]
		if found {
			if paused, _ := strconv.ParseBool(annoStr); paused {
				return controllerutil.OperationResultNone, nil
			}
		}
		// Upgrade any pre-SSA (Update-operation) ownership left by the old
		// reconciler into our Apply manager so the following apply can prune
		// fields we no longer declare. Idempotent: a no-op once migrated.
		if err := migrateLegacyManagedFields(ctx, cli, current); err != nil {
			return controllerutil.OperationResultNone, err
		}
		resourceVersion = current.GetResourceVersion()
	}

	gvks, _, err := cli.Scheme().ObjectKinds(obj)
	if err != nil {
		return controllerutil.OperationResultNone, err
	}
	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	obj.SetResourceVersion("")
	obj.SetManagedFields(nil)

	var fnErr error
	for _, f := range fn {
		fnErr = errors.Join(fnErr, f(obj))
	}
	if fnErr != nil {
		return controllerutil.OperationResultNone, fnErr
	}

	// SSA Apply requires a Name; GenerateName objects must use Create.
	if obj.GetName() == "" && obj.GetGenerateName() != "" {
		if err = cli.Create(ctx, obj); err != nil {
			return controllerutil.OperationResultNone, err
		}
		return controllerutil.OperationResultCreated, nil
	}

	if err = cli.Patch(ctx, obj, client.Apply, client.FieldOwner(FieldManager), client.ForceOwnership); err != nil {
		return controllerutil.OperationResultNone, err
	}

	if !exists {
		return controllerutil.OperationResultCreated, nil
	}
	// A no-op apply leaves resourceVersion untouched; a bump means the object
	// actually changed. This is reliable across the apply round-trip, unlike
	// comparing the object the server mutated (RV, timestamps, managedFields).
	if obj.GetResourceVersion() == resourceVersion {
		return controllerutil.OperationResultNone, nil
	}
	return controllerutil.OperationResultUpdated, nil
}
