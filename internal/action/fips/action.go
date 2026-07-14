package fips

import (
	"context"
	"fmt"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewAction creates a generic FIPS validation action that checks crypto
// material and password refs before the signer action runs.
func NewAction[T apis.ConditionsAwareObject](
	conditionType string,
	component string,
	wrapper func(T) *wrapper[T],
) action.Action[T] {
	return &fipsAction[T]{
		conditionType: conditionType,
		component:     component,
		wrapper:       wrapper,
	}
}

type fipsAction[T apis.ConditionsAwareObject] struct {
	action.BaseAction
	conditionType string
	component     string
	wrapper       func(T) *wrapper[T]
}

func (i fipsAction[T]) Name() string {
	return fmt.Sprintf("validate %s FIPS compliance", i.component)
}

func (i fipsAction[T]) CanHandle(_ context.Context, instance T) bool {
	if !fipsutil.Enabled() {
		return false
	}

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

func (i fipsAction[T]) Handle(ctx context.Context, instance T) *action.Result {
	w := i.wrapper(instance)

	if w.PasswordRef() != nil {
		err := reconcile.TerminalError(fipsutil.ErrPasswordRefInFIPS)
		return i.Error(ctx, err, instance,
			metav1.Condition{
				Type:               i.conditionType,
				Status:             metav1.ConditionFalse,
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.GetGeneration(),
			},
		)
	}

	if err := i.validateCryptoMaterial(ctx, w); err != nil {
		if fipsutil.IsValidationError(err) {
			return i.Error(ctx, reconcile.TerminalError(err), instance,
				metav1.Condition{
					Type:               i.conditionType,
					Status:             metav1.ConditionFalse,
					Reason:             state.Failure.String(),
					Message:            err.Error(),
					ObservedGeneration: instance.GetGeneration(),
				},
			)
		}
		return i.Continue()
	}

	instance.SetCondition(metav1.Condition{
		Type:               i.conditionType,
		Status:             metav1.ConditionTrue,
		Reason:             "FIPSValid",
		Message:            "All crypto material is FIPS-compliant",
		ObservedGeneration: instance.GetGeneration(),
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

func (i fipsAction[T]) validateCryptoMaterial(ctx context.Context, w *wrapper[T]) error {
	refs, err := w.CryptoMaterial(ctx, i.Client)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if err := ref.Validate(ref.Data); err != nil {
			return fmt.Errorf("FIPS validation failed for %s: %w", ref.FieldPath, err)
		}
	}
	return nil
}
