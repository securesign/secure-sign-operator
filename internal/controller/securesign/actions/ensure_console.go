package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/annotations"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewConsoleAction() action.Action[*rhtasv1alpha1.Securesign] {
	return &consoleAction{}
}

type consoleAction struct {
	action.BaseAction
}

func (i consoleAction) Name() string {
	return "create console"
}

func (i consoleAction) CanHandle(context.Context, *rhtasv1alpha1.Securesign) bool {
	return true
}

func (i consoleAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Securesign) *action.Result {
	var (
		err     error
		result  controllerutil.OperationResult
		l       = labels.For("console", instance.Name, instance.Name)
		console = &rhtasv1alpha1.Console{
			ObjectMeta: v1.ObjectMeta{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		}
	)

	if result, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		console,
		ensure.ControllerReference[*rhtasv1alpha1.Console](instance, i.Client),
		ensure.Labels[*rhtasv1alpha1.Console](slices.Collect(maps.Keys(l)), l),
		ensure.Annotations[*rhtasv1alpha1.Console](annotations.InheritableAnnotations, instance.Annotations),
		func(object *rhtasv1alpha1.Console) error {
			object.Spec = *instance.Spec.Console
			return nil
		},
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Console: %w", err), instance,
			v1.Condition{
				Type:    ConsoleCondition,
				Status:  v1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
	}

	if result != controllerutil.OperationResultNone {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    ConsoleCondition,
			Status:  v1.ConditionFalse,
			Reason:  state.Creating.String(),
			Message: "Console resource created " + console.Name,
		})
		return i.StatusUpdate(ctx, instance)
	}

	return i.CopyStatus(ctx, console, instance)
}

func (i consoleAction) CopyStatus(ctx context.Context, object *rhtasv1alpha1.Console, instance *rhtasv1alpha1.Securesign) *action.Result {
	objectStatus := meta.FindStatusCondition(object.Status.Conditions, constants.ReadyCondition)
	if objectStatus == nil {
		// not initialized yet, wait for update
		return i.Continue()
	}
	if meta.FindStatusCondition(instance.Status.Conditions, ConsoleCondition).Reason != objectStatus.Reason {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   ConsoleCondition,
			Status: objectStatus.Status,
			Reason: objectStatus.Reason,
		})
		return i.StatusUpdate(ctx, instance)
	}
	return i.Continue()
}
