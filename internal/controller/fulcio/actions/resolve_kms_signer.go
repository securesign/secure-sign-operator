package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/generateSigner"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type resolveKMSSignerAction struct {
	action.BaseAction
}

func NewResolveKMSSignerAction() action.Action[*rhtasv1.Fulcio] {
	return &resolveKMSSignerAction{}
}

func (a *resolveKMSSignerAction) Name() string {
	return "resolve fulcio KMS signer"
}

func (a *resolveKMSSignerAction) CanHandle(_ context.Context, instance *rhtasv1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Pending:
		return false
	}

	cc := meta.FindStatusCondition(instance.GetConditions(), CertCondition)
	return instance.Spec.Signer.Type == rhtasv1.SignerTypeKMS &&
		(cc == nil || cc.Status != metav1.ConditionTrue || instance.GetGeneration() != cc.ObservedGeneration)
}

func (a *resolveKMSSignerAction) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	chainRef := instance.Spec.Signer.CertificateChain.CertificateChainRef
	if err := generateSigner.RequireSecret(ctx, a.Client, instance.Namespace, chainRef); err != nil {
		return a.Error(ctx, fmt.Errorf("certificate chain secret: %w", err), instance,
			metav1.Condition{
				Type:               CertCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.GetGeneration(),
			},
			metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Pending.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.GetGeneration(),
			},
		)
	}

	if err := a.ensureCertChainLabel(ctx, instance, chainRef); err != nil {
		a.Logger.V(1).Info("failed to apply labels to certificate chain secret", "error", err)
	}

	instance.Status.Certificate = &rhtasv1.FulcioCertStatus{
		CARef: chainRef.DeepCopy(),
	}

	instance.SetCondition(metav1.Condition{
		Type:               CertCondition,
		Status:             metav1.ConditionTrue,
		Reason:             constants.ReasonResolved,
		Message:            "Using existing secret",
		ObservedGeneration: instance.GetGeneration(),
	})

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}

func (a *resolveKMSSignerAction) ensureCertChainLabel(ctx context.Context, instance *rhtasv1.Fulcio, ref *rhtasv1.SecretKeySelector) error {
	return kubernetes.EnsureExclusiveSecretLabel(ctx, a.Client, instance.Namespace, ref.Name, FulcioCALabel, constants.KeyCert)
}
