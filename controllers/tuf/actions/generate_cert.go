package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	v12 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewPendingAction() action.Action[rhtasv1alpha1.Tuf] {
	return &pendingAction{}
}

type pendingAction struct {
	action.BaseAction
}

func (i pendingAction) Name() string {
	return "pending"
}

func (i pendingAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	return tuf.Status.Phase == rhtasv1alpha1.PhaseNone || tuf.Status.Phase == rhtasv1alpha1.PhasePending
}

func (i pendingAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	if instance.Status.Phase == rhtasv1alpha1.PhaseNone {
		instance.Status.Phase = rhtasv1alpha1.PhasePending
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:    string(rhtasv1alpha1.PhaseReady),
			Status:  v1.ConditionFalse,
			Reason:  (string)(rhtasv1alpha1.PhasePending),
			Message: "Resolving keys",
		})
		return i.StatusUpdate(ctx, instance)
	}

	for index, key := range instance.Spec.Keys {
		if meta.FindStatusCondition(instance.Status.Conditions, key.Name) == nil {
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:   key.Name,
				Status: v1.ConditionUnknown,
				Reason: "Resolving",
			})
			return i.StatusUpdate(ctx, instance)
		}

		if !meta.IsStatusConditionTrue(instance.Status.Conditions, key.Name) {
			updated, err := i.handleKey(ctx, instance, &instance.Spec.Keys[index])
			if err != nil {
				if !meta.IsStatusConditionFalse(instance.Status.Conditions, key.Name) {
					meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
						Type:    key.Name,
						Status:  v1.ConditionFalse,
						Reason:  "Failure",
						Message: err.Error(),
					})
					return i.StatusUpdate(ctx, instance)
				}

				// swallow error and retry
				return i.Requeue()
			}
			if updated {
				return i.Update(ctx, instance)
			}

			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:   key.Name,
				Status: v1.ConditionTrue,
				Reason: "Ready",
			})
			return i.StatusUpdate(ctx, instance)
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: string(rhtasv1alpha1.PhaseReady),
		Status: v1.ConditionTrue, Reason: string(rhtasv1alpha1.PhaseCreating)})
	instance.Status.Phase = rhtasv1alpha1.PhaseCreating
	return i.StatusUpdate(ctx, instance)
}

func (i pendingAction) handleKey(ctx context.Context, instance *rhtasv1alpha1.Tuf, key *rhtasv1alpha1.TufKey) (bool, error) {
	switch {
	case key.SecretRef == nil:
		sks, err := i.discoverSecret(ctx, instance.Namespace, key)
		if err != nil {
			return false, err
		}
		key.SecretRef = sks
		return true, nil
	case key.SecretRef != nil:
		return false, nil
	default:
		return false, errors.New(fmt.Sprintf("Unable to resolve %s key. Enable autodiscovery or set secret reference.", key.Name))
	}
}

func (i pendingAction) discoverSecret(ctx context.Context, namespace string, key *rhtasv1alpha1.TufKey) (*rhtasv1alpha1.SecretKeySelector, error) {
	labelName := constants.TufLabelNamespace + "/" + key.Name
	s, err := k8sutils.FindSecret(ctx, i.Client, namespace, labelName)
	if err != nil {
		return nil, err
	}
	if s != nil {
		keySelector := s.Labels[labelName]
		if keySelector == "" {
			err = errors.New(fmt.Sprintf("label %s is empty", labelName))
			return nil, err
		}
		return &rhtasv1alpha1.SecretKeySelector{
			Key: keySelector,
			LocalObjectReference: v12.LocalObjectReference{
				Name: s.Name,
			},
		}, nil
	}

	return nil, errors.New("secret not found")
}
