package actions

import (
	"context"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewHandleErrorAction() action.Action[rhtasv1alpha1.CTlog] {
	return &handleErrorAction{}
}

type handleErrorAction struct {
	action.BaseAction
}

func (i handleErrorAction) Name() string {
	return "error handler"
}

func (i handleErrorAction) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c == nil {
		return false
	}
	return c.Reason == constants.Failure && instance.Status.Restarts < constants.AllowedRestarts
}

func (i handleErrorAction) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	i.Recorder.Event(instance, v1.EventTypeWarning, constants.Failure, "Restarted by error handler")

	newStatus := rhtasv1alpha1.CTlogStatus{}

	newStatus.Restarts = instance.Status.Restarts + 1
	if newStatus.Restarts == constants.AllowedRestarts {
		meta.SetStatusCondition(&newStatus.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: "Restart threshold reached",
		})
		instance.Status = newStatus
		return i.StatusUpdate(ctx, instance)
	}

	// - keep the status.treeId if not nil
	newStatus.TreeID = instance.Status.TreeID

	newStatus.PrivateKeyRef = instance.Status.PrivateKeyRef.DeepCopy()

	newStatus.PublicKeyRef = instance.Status.PublicKeyRef.DeepCopy()

	newStatus.PrivateKeyPasswordRef = instance.Status.PrivateKeyPasswordRef.DeepCopy()

	if meta.IsStatusConditionTrue(instance.Status.Conditions, CertCondition) {
		copy(newStatus.RootCertificates, instance.Status.RootCertificates)
	}

	if meta.IsStatusConditionTrue(instance.Status.Conditions, ServerCondition) {
		instance.Status.ServerConfigRef.DeepCopyInto(newStatus.ServerConfigRef)
		// do not append server condition - let controller to redeploy
	}

	meta.SetStatusCondition(&newStatus.Conditions, metav1.Condition{
		Type:    constants.Ready,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Pending,
		Message: "Restarted by error handler",
	})
	instance.Status = newStatus
	return i.StatusUpdate(ctx, instance)
}
