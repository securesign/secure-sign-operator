package actions

import (
	"context"
	"encoding/json"
	"reflect"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/state"
	"gomodules.xyz/jsonpatch/v2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func NewTrustRootSyncAction() action.Action[*rhtasv1.Tuf] {
	return &trustRootSyncAction{}
}

type trustRootSyncAction struct {
	action.BaseAction
}

func (i trustRootSyncAction) Name() string {
	return "synchronize trust root repository"
}

func (i trustRootSyncAction) CanHandle(_ context.Context, instance *rhtasv1.Tuf) bool {
	return state.FromInstance(instance, constants.ReadyCondition) == state.Ready &&
		instance.Annotations[tufConstants.RepositoryVersionAnnotation] == tufConstants.TufVersionV1
}

func (i trustRootSyncAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	specComponents := rhtasv1.Components{
		Ctlog:  instance.Spec.Ctlog,
		Fulcio: instance.Spec.Fulcio,
		Rekor:  instance.Spec.Rekor,
		Tsa:    ptr.Deref(instance.Spec.Tsa, nil),
	}
	desired, err := trustroot.Resolve(ctx, i.Client, instance.Namespace, specComponents)
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	actual, err := trustroot.Resolve(ctx, i.Client, instance.Namespace, ptr.Deref(instance.Status.TrustRoot.Published, rhtasv1.Components{}))
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	if reflect.DeepEqual(desired, actual) {
		return i.Continue()
	}

	// generate the diff
	patch, err := createJSONPatch(actual, desired)
	if err != nil {
		return i.Error(ctx, err, instance)
	}

	// start a job to synchronize the trust root
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               tufConstants.TrustRootSynced,
		Status:             metav1.ConditionFalse,
		Reason:             tufConstants.SyncInProgress,
		Message:            "TUF trust root synchronization in progress " + string(patch), // add job-name ref
		ObservedGeneration: instance.Generation,
	})

	// wait for the job to complete
	// set condition to true if the job completed successfully
	// copy all trustmaterial from ref-services into secrets
	// set status.trustroot.published to the new trust root

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

func createJSONPatch(actual, desired trustroot.ResolvedComponents) ([]byte, error) {
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		return nil, err
	}

	desiredJSON, err := json.Marshal(desired)
	if err != nil {
		return nil, err
	}

	patch, err := jsonpatch.CreatePatch(actualJSON, desiredJSON)
	if err != nil {
		return nil, err
	}
	return json.Marshal(patch)
}
