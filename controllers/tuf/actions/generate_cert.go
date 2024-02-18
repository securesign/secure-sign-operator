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

func NewResolveKeysAction() action.Action[rhtasv1alpha1.Tuf] {
	return &resolveKeysAction{}
}

type resolveKeysAction struct {
	action.BaseAction
}

func (i resolveKeysAction) Name() string {
	return "resolve keys"
}

func (i resolveKeysAction) CanHandle(tuf *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(tuf.Status.Conditions, constants.Ready)
	return c.Reason == constants.Pending
}

func (i resolveKeysAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	for index, key := range instance.Spec.Keys {
		if !meta.IsStatusConditionTrue(instance.Status.Conditions, key.Name) {
			updated, err := i.handleKey(ctx, instance, &instance.Spec.Keys[index])
			if err != nil {
				meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.Ready,
					Status: v1.ConditionFalse, Reason: constants.Pending, Message: "Resolving keys"})
				meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
					Type:    key.Name,
					Status:  v1.ConditionFalse,
					Reason:  constants.Failure,
					Message: err.Error(),
				})
				i.StatusUpdate(ctx, instance)
				return i.Requeue()
			}
			if updated {
				return i.Update(ctx, instance)
			}

			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:   key.Name,
				Status: v1.ConditionTrue,
				Reason: constants.Ready,
			})
			return i.StatusUpdate(ctx, instance)
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.Ready,
		Status: v1.ConditionFalse, Reason: constants.Creating, Message: "Keys resolved"})
	return i.StatusUpdate(ctx, instance)
}

func (i resolveKeysAction) handleKey(ctx context.Context, instance *rhtasv1alpha1.Tuf, key *rhtasv1alpha1.TufKey) (bool, error) {
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

func (i resolveKeysAction) discoverSecret(ctx context.Context, namespace string, key *rhtasv1alpha1.TufKey) (*rhtasv1alpha1.SecretKeySelector, error) {
	labelName := constants.LabelNamespace + "/" + key.Name
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
