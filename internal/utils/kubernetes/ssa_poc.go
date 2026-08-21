package kubernetes

// POC: shows how CreateOrUpdate could be replaced by Server-Side Apply
// without rewriting the existing ensure.* functions. Not wired into any
// controller.
//
// Unlike CreateOrUpdate, fn does NOT run against the live object -- obj must
// be bare (TypeMeta + Name/Namespace only). SSA claims ownership of every
// field present in the patch, so reusing a Get'd copy would try to take over
// fields owned by other controllers. Because of that, any field fn omits is
// released by the API server on Apply, which is why the per-component
// label/annotation pruning closures (duplicated across ingress.go x5)
// wouldn't be needed anymore.
//
// client.Apply (raw-struct SSA patch) is deprecated in favor of
// cli.Apply(ctx, applyConfig, ...) with generated *ApplyConfiguration types,
// but it lets fn keep mutating the real API type -- no ensure.* rewrite.

import (
	"context"
	"strconv"

	"github.com/securesign/operator/internal/annotations"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// FieldOwner is the single SSA field manager used by every ApplySSA call
// across the operator. Kept as one constant, not one per resource/component,
// so the operator's own writes never conflict with each other.
const FieldOwner = client.FieldOwner("secure-sign-operator")

// ApplySSA returns changed=true when the patch actually altered the object
// (created or updated), so callers can gate status updates the same way they
// did on CreateOrUpdate's OperationResult -- skip that and every action
// sharing a condition type will fight over its Message on every reconcile.
func ApplySSA[T client.Object](ctx context.Context, cli client.Client, obj T, fn ...func(T) error) (changed bool, err error) {
	// Raw client.Apply marshals obj's Go struct directly (no scheme lookup
	// at patch time), so TypeMeta must be set explicitly or the server
	// rejects the patch.
	gvk, err := apiutil.GVKForObject(obj, cli.Scheme())
	if err != nil {
		return false, err
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)

	// SSA needs no prior state, but the pause-reconciliation guard and the
	// change signal below both do -- one cheap read covers both.
	current := obj.DeepCopyObject().(T)
	isCreate := false
	switch err := cli.Get(ctx, client.ObjectKeyFromObject(obj), current); {
	case apierrors.IsNotFound(err):
		isCreate = true
	case err != nil:
		return false, err
	default:
		if paused, _ := strconv.ParseBool(current.GetAnnotations()[annotations.PausedReconciliation]); paused {
			return false, nil
		}
	}

	for _, f := range fn {
		if err := f(obj); err != nil {
			return false, err
		}
	}

	if err := cli.Patch(ctx, obj, client.Apply, FieldOwner, client.ForceOwnership); err != nil {
		return false, err
	}

	// A no-op Apply doesn't bump resourceVersion; a real create/update does.
	return isCreate || obj.GetResourceVersion() != current.GetResourceVersion(), nil
}
