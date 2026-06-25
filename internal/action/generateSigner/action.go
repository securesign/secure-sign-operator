package generateSigner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"strings"
	"slices"
	"sort"
	"time"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	reasonResolved = "Resolved"
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

	if w.Resolve(ctx, i.Client) {
		instance.SetCondition(metav1.Condition{
			Type:               i.conditionType,
			Status:             metav1.ConditionTrue,
			Reason:             reasonResolved,
			Message:            "Using existing secret",
			ObservedGeneration: instance.GetGeneration(),
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	secretName := fmt.Sprintf(i.secretNameFormat, instance.GetName())
	nn := types.NamespacedName{Name: secretName, Namespace: instance.GetNamespace()}

	existing := &corev1.Secret{}
	err := i.Client.Get(ctx, nn, existing)

	switch {
	case err == nil:
		return i.handleExisting(ctx, instance, w, existing)
	case apierrors.IsNotFound(err):
		return i.handleCreate(ctx, instance, w, secretName)
	default:
		return i.Error(ctx, fmt.Errorf("failed to get signer secret %q: %w", secretName, err), instance)
	}
}

func (i signerAction[T]) handleExisting(ctx context.Context, instance T, w *wrapper[T], existing *corev1.Secret) *action.Result {
	storedHash := existing.Annotations[annotations.DataHash]
	actualHash := ComputeDataHash(existing.Data)
	if storedHash != "" && storedHash != actualHash {
		return i.Error(ctx,
			reconcile.TerminalError(ErrDataTampered(existing.Name, storedHash, actualHash)),
			instance, metav1.Condition{
				Type:               i.conditionType,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            ErrDataTampered(existing.Name, storedHash, actualHash).Error(),
				ObservedGeneration: instance.GetGeneration(),
			})
	}

	w.AlignStatus(existing)
	instance.SetCondition(metav1.Condition{
		Type:               i.conditionType,
		Status:             metav1.ConditionTrue,
		Reason:             reasonResolved,
		ObservedGeneration: instance.GetGeneration(),
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
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

	componentLabels := labels.For(i.component, i.deployment, instance.GetName())
	hashAnnotations := map[string]string{
		annotations.DataHash: ComputeDataHash(data),
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: instance.GetNamespace(),
		},
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		secret,
		ensure.Labels[*corev1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
		ensure.Annotations[*corev1.Secret](slices.Collect(maps.Keys(hashAnnotations)), hashAnnotations),
		ensure.ControllerReference[*corev1.Secret](instance, i.Client),
		kubernetes.EnsureSecretData(true, data),
		w.EnsureMutate(),
	); err != nil {
		if isImmutableFieldError(err) {
			return i.Error(ctx,
				reconcile.TerminalError(ErrConfigMismatch(secretName)),
				instance, metav1.Condition{
					Type:               i.conditionType,
					Status:             metav1.ConditionFalse,
					Reason:             state.Failure.String(),
					Message:            err.Error(),
					ObservedGeneration: instance.GetGeneration(),
				})
		}
		return i.Error(ctx, fmt.Errorf("failed to create signer secret: %w", err), instance,
			metav1.Condition{
				Type:               i.conditionType,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.GetGeneration(),
			})
	}

	i.Recorder.Eventf(instance, secret, corev1.EventTypeNormal, "SignerSecretCreated", "Created",
		"Created immutable signer secret %s", secret.Name)

	w.AlignStatus(secret)
	instance.SetCondition(metav1.Condition{
		Type:               i.conditionType,
		Status:             metav1.ConditionTrue,
		Reason:             reasonResolved,
		ObservedGeneration: instance.GetGeneration(),
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

// ComputeDataHash computes a deterministic SHA256 hash of secret data.
// Keys are sorted to ensure consistent hashing regardless of map iteration order.
func ComputeDataHash(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write(data[k])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// isImmutableFieldError checks if the error is a Kubernetes API server rejection
// for attempting to update an immutable secret's data field.
func isImmutableFieldError(err error) bool {
	if !apierrors.IsInvalid(err) {
		return false
	}
	if statusErr, ok := err.(*apierrors.StatusError); ok {
		for _, cause := range statusErr.Status().Details.Causes {
			if cause.Type == metav1.CauseTypeForbidden &&
				strings.Contains(cause.Message, "field is immutable") {
				return true
			}
		}
	}
	return false
}

