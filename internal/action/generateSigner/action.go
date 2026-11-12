package generateSigner

import (
	"context"
	"errors"
	"fmt"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewAction creates a generic signer secret action.
func NewAction[T apis.ConditionsAwareObject](
	conditionType string,
	secretNameFormat string,
	component string,
	deployment string,
	wrapper func(T) *wrapper[T],
) action.Action[T] {
	return &signerAction[T]{
		conditionType:    conditionType,
		secretNameFormat: secretNameFormat,
		component:        component,
		deployment:       deployment,
		wrapper:          wrapper,
	}
}

type signerAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	conditionType    string
	secretNameFormat string
	component        string
	deployment       string
	wrapper          func(T) *wrapper[T]
}

func (i signerAction[T]) Name() string {
	return fmt.Sprintf("resolve %s signer secret", i.component)
}

func (i signerAction[T]) CanHandle(_ context.Context, instance T) bool {
	w := i.wrapper(instance)
	c := meta.FindStatusCondition(instance.GetConditions(), constants.ReadyCondition)

	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Pending:
		return false
	case !w.IsEnabled():
		return false
	default:
		cc := meta.FindStatusCondition(instance.GetConditions(), i.conditionType)
		return cc == nil || cc.Status != metav1.ConditionTrue || instance.GetGeneration() != cc.ObservedGeneration
	}
}

func (i signerAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	w := i.wrapper(instance)

	resolvedRef, err := w.ResolveRef(ctx, i.Client)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("%w: %w", ErrResolveFailed, err), instance,
			metav1.Condition{
				Type:               i.conditionType,
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
	if resolvedRef != nil {
		// TODO: replace with a dedicated resolve_pub_key action per component
		// that fetches public keys from the running service API.
		if err := EnsureDiscoveryLabels(ctx, i.Client, instance.GetNamespace(), resolvedRef.Name, w.cfg.DiscoveryLabels); err != nil {
			i.Logger.V(1).Info("failed to apply labels to user-provided secret", "error", err)
		}
		w.AlignStatus(*resolvedRef)
		instance.SetCondition(metav1.Condition{
			Type:               i.conditionType,
			Status:             metav1.ConditionTrue,
			Reason:             constants.ReasonResolved,
			Message:            "Using existing secret",
			ObservedGeneration: instance.GetGeneration(),
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	deterministicName := fmt.Sprintf(i.secretNameFormat, instance.GetName())

	found, err := kubernetes.ExistsSecret(ctx, i.Client, instance.GetNamespace(), deterministicName)

	switch {
	case err != nil:
		return i.Error(ctx, fmt.Errorf("%w %q: %w", ErrSecretGet, deterministicName, err), instance)
	case found:
		if err := EnsureDiscoveryLabels(ctx, i.Client, instance.GetNamespace(), deterministicName, w.cfg.DiscoveryLabels); err != nil {
			return i.Error(ctx, fmt.Errorf("failed to apply discovery labels to existing secret: %w", err), instance)
		}
		w.AlignStatus(rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: deterministicName}})
		instance.SetCondition(metav1.Condition{
			Type:               i.conditionType,
			Status:             metav1.ConditionTrue,
			Reason:             constants.ReasonResolved,
			ObservedGeneration: instance.GetGeneration(),
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	default:
		return i.handleCreate(ctx, instance, w, deterministicName)
	}
}

func (i signerAction[T]) handleCreate(ctx context.Context, instance T, w *wrapper[T], secretName string) *action.Result {
	data, err := w.GenerateData(ctx, i.Client)
	if err != nil {
		if errors.Is(err, reconcile.TerminalError(err)) {
			return i.Error(ctx, err, instance, metav1.Condition{
				Type:               i.conditionType,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.GetGeneration(),
			})
		}
		i.Logger.Error(err, "error generating signer data")
		instance.SetCondition(metav1.Condition{
			Type:               i.conditionType,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            err.Error(),
			ObservedGeneration: instance.GetGeneration(),
		})
		if _, persistErr := i.PersistStatus(ctx, instance); persistErr != nil {
			return i.Error(ctx, persistErr, instance)
		}
		return i.RequeueAfter(5 * time.Second)
	}

	secret, err := traits.Apply(corev1ac.Secret(secretName, instance.GetNamespace()).WithImmutable(true).WithData(data),
		traits.Labels[*corev1ac.SecretApplyConfiguration](labels.For(i.component, i.deployment, instance.GetName())))
	if err != nil {
		return i.Error(ctx, fmt.Errorf("failed to build signer secret: %w", err), instance)
	}

	live := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.GetNamespace(),
		},
	}
	_, err = kubernetes.Apply(ctx, i.Client, secret, live)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("%w: %w", ErrSecretCreate, err), instance,
			metav1.Condition{
				Type:               i.conditionType,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.GetGeneration(),
			})
	}

	i.Recorder.Eventf(instance, live, corev1.EventTypeNormal, "SignerSecretCreated", "Created",
		"Created immutable signer secret %s", live.Name)

	if err := EnsureDiscoveryLabels(ctx, i.Client, instance.GetNamespace(), secretName, w.cfg.DiscoveryLabels); err != nil {
		return i.Error(ctx, fmt.Errorf("failed to apply discovery labels to signer secret: %w", err), instance)
	}

	w.AlignStatus(rhtasv1.SecretKeySelector{LocalObjectReference: rhtasv1.LocalObjectReference{Name: secretName}})
	instance.SetCondition(metav1.Condition{
		Type:               i.conditionType,
		Status:             metav1.ConditionTrue,
		Reason:             constants.ReasonResolved,
		ObservedGeneration: instance.GetGeneration(),
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
