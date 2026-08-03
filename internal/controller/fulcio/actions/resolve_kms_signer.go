package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/generateSigner"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
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
	if instance.Spec.Signer.Type != rhtasv1.FulcioSignerTypeKMS {
		return false
	}

	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)
	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Pending:
		return false
	}

	cc := meta.FindStatusCondition(instance.GetConditions(), CertCondition)
	return cc == nil || cc.Status != metav1.ConditionTrue || instance.GetGeneration() != cc.ObservedGeneration
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
		Reason:             "Resolved",
		Message:            "Using existing secret",
		ObservedGeneration: instance.GetGeneration(),
	})

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}

func (a *resolveKMSSignerAction) ensureCertChainLabel(ctx context.Context, instance *rhtasv1.Fulcio, ref *rhtasv1.SecretKeySelector) error {
	label := FulcioCALabel

	existing, err := kubernetes.ListSecrets(ctx, a.Client, instance.Namespace, label)
	if err != nil {
		return err
	}
	for _, s := range existing.Items {
		if s.Name == ref.Name {
			continue
		}
		if err := labels.Remove(ctx, &s, a.Client, label); err != nil {
			return err
		}
	}

	secret := &corev1.Secret{}
	secret.Name = ref.Name
	secret.Namespace = instance.Namespace
	_, err = kubernetes.CreateOrUpdate(ctx, a.Client, secret,
		ensure.Labels[*corev1.Secret]([]string{label}, map[string]string{label: constants.KeyCert}),
	)
	return err
}
