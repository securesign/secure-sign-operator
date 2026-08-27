package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/generateSigner"
	"github.com/securesign/operator/internal/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var ErrMissingKeysetRef = errors.New("missing keyset reference for Tink signer")

type resolveKMSTinkSignerAction struct {
	action.BaseAction
}

func NewResolveKMSTinkSignerAction() action.Action[*rhtasv1.TimestampAuthority] {
	return &resolveKMSTinkSignerAction{}
}

func (a *resolveKMSTinkSignerAction) Name() string {
	return "resolve timestamp-authority KMS/Tink signer"
}

func (a *resolveKMSTinkSignerAction) CanHandle(_ context.Context, instance *rhtasv1.TimestampAuthority) bool {
	if tsaUtils.IsFileType(instance) {
		return false
	}

	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Pending:
		return false
	}

	cc := meta.FindStatusCondition(instance.GetConditions(), TSASignerCondition)
	return cc == nil || cc.Status != metav1.ConditionTrue || instance.GetGeneration() != cc.ObservedGeneration
}

func (a *resolveKMSTinkSignerAction) Handle(ctx context.Context, instance *rhtasv1.TimestampAuthority) *action.Result {
	signer := &instance.Spec.Signer

	if signer.Tink != nil {
		if signer.Tink.KeysetRef == nil {
			return a.Error(ctx, reconcile.TerminalError(ErrMissingKeysetRef), instance,
				metav1.Condition{
					Type:               TSASignerCondition,
					Status:             metav1.ConditionFalse,
					Reason:             state.Failure.String(),
					Message:            ErrMissingKeysetRef.Error(),
					ObservedGeneration: instance.GetGeneration(),
				},
			)
		}
		if err := generateSigner.RequireSecret(ctx, a.Client, instance.Namespace, signer.Tink.KeysetRef); err != nil {
			return a.Error(ctx, fmt.Errorf("tink keyset secret: %w", err), instance,
				metav1.Condition{
					Type:               TSASignerCondition,
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
	}

	chainRef := signer.CertificateChain.CertificateChainRef
	if err := generateSigner.RequireSecret(ctx, a.Client, instance.Namespace, chainRef); err != nil {
		return a.Error(ctx, fmt.Errorf("certificate chain secret: %w", err), instance,
			metav1.Condition{
				Type:               TSASignerCondition,
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

	instance.Status.Signer = &rhtasv1.TimestampAuthoritySignerStatus{
		CertificateChainRef: chainRef.DeepCopy(),
	}

	instance.SetCondition(metav1.Condition{
		Type:               TSASignerCondition,
		Status:             metav1.ConditionTrue,
		Reason:             constants.ReasonResolved,
		Message:            "Using existing secret",
		ObservedGeneration: instance.GetGeneration(),
	})

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}

func (a *resolveKMSTinkSignerAction) ensureCertChainLabel(ctx context.Context, instance *rhtasv1.TimestampAuthority, ref *rhtasv1.SecretKeySelector) error {
	label := labels.LabelNamespace + "/tsa.certchain.pem"
	return kubernetes.EnsureExclusiveSecretLabel(ctx, a.Client, instance.Namespace, ref.Name, label, tsaUtils.KeyCertificateChain)
}
