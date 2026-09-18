package server

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type resolveKMSSignerAction struct {
	action.BaseAction
}

const kmsSignerStatusMessage = "Using KMS signer"

func NewResolveKMSSignerAction() action.Action[*rhtasv1.Rekor] {
	return &resolveKMSSignerAction{}
}

func (a *resolveKMSSignerAction) Name() string {
	return "resolve rekor KMS signer"
}

func (a *resolveKMSSignerAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	if instance.Spec.Signer.Type != rhtasv1.SignerTypeKMS {
		return false
	}

	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Pending:
		return false
	}

	cc := meta.FindStatusCondition(instance.GetConditions(), actions.SignerCondition)
	return instance.Status.Signer.KeyRef != nil || instance.Status.Signer.PasswordRef != nil ||
		cc == nil || cc.Status != metav1.ConditionTrue || instance.GetGeneration() != cc.ObservedGeneration
}

func (a *resolveKMSSignerAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	instance.Status.Signer = rhtasv1.RekorSignerStatus{}

	instance.SetCondition(metav1.Condition{
		Type:               actions.SignerCondition,
		Status:             metav1.ConditionTrue,
		Reason:             constants.ReasonResolved,
		Message:            kmsSignerStatusMessage,
		ObservedGeneration: instance.GetGeneration(),
	})

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}
