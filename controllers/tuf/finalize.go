package tuf

import (
	"context"
	"fmt"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewFinalizeAction() action.Action[rhtasv1alpha1.Tuf] {
	return &finalizeAction{}
}

type finalizeAction struct {
	action.BaseAction
}

func (i *finalizeAction) Name() string {
	return "finalize"
}

func (i *finalizeAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	// Check if the Tuf instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set.
	return tuf.GetDeletionTimestamp() != nil && controllerutil.ContainsFinalizer(tuf, finalizer)
}

func (i *finalizeAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) (*rhtasv1alpha1.Tuf, error) {
	var err error
	// Run finalization logic for tufFinalizer. If the
	// finalization logic fails, don't remove the finalizer so
	// that we can retry during the next reconciliation.

	i.Logger.Info("Performing Finalizer Operations for TUF before delete CR")

	// Let's add here a status "Downgrade" to define that this resource begin its process to be terminated.
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    string(rhtasv1alpha1.PhaseReady),
		Status:  metav1.ConditionUnknown,
		Reason:  "Finalizing",
		Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", instance.Name)})

	// Perform all operations required before remove the finalizer and allow
	// the Kubernetes API to remove the custom resource.
	_ = i.doFinalizerOperations(ctx, instance)

	// TODO(user): If you add operations to the doFinalizerOperations method
	// then you need to ensure that all worked fine before deleting and updating the Downgrade status
	// otherwise, you should requeue here.

	// Re-fetch the Tuf Custom Resource before update the status
	// so that we have the latest state of the resource on the cluster and we will avoid
	// raise the issue "the object has been modified, please apply
	// your changes to the latest version and try again" which would re-trigger the reconciliation
	nn := types.NamespacedName{
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}
	if err = i.Client.Get(ctx, nn, instance); err != nil {
		i.Logger.Error(err, "Failed to re-fetch TUF")
		return instance, err
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    string(rhtasv1alpha1.PhaseReady),
		Status:  metav1.ConditionTrue,
		Reason:  "Finalizing",
		Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", instance.Name)})

	i.Logger.Info("Removing Finalizer for TUF after successfully perform the operations")
	if ok := controllerutil.RemoveFinalizer(instance, finalizer); !ok {
		i.Logger.Error(err, "Failed to remove finalizer for TUF")
		return instance, nil
	}
	if err = i.Client.Update(ctx, instance); err != nil {
		i.Logger.Error(err, "Failed to remove finalizer for TUF")
		return instance, err
	}

	return instance, nil
}

func (i *finalizeAction) doFinalizerOperations(ctx context.Context, instance *rhtasv1alpha1.Tuf) error {
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	// Note: It is not recommended to use finalizers with the purpose of delete resources which are
	// created and managed in the reconciliation. These ones, such as the Deployment created on this reconcile,
	// are defined as depended of the custom resource. See that we use the method ctrl.SetControllerReference.
	// to set the ownerRef which means that the Deployment will be deleted by the Kubernetes API.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/

	// The following implementation will raise an event
	i.Recorder.Event(instance, "Warning", "Deleting",
		fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s",
			instance.Name,
			instance.Namespace))
	return nil
}
