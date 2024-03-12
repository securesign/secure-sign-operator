package actions

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	k8sutils "github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	"k8s.io/apimachinery/pkg/api/equality"
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

func (i resolveKeysAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Tuf) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c.Reason != constants.Pending && c.Reason != constants.Ready {
		return false
	}

	if !equality.Semantic.DeepDerivative(instance.Spec.Keys, instance.Status.Keys) {
		return true
	}
	for index, k := range instance.Spec.Keys {
		if k.SecretRef == nil {
			if scr, _ := k8sutils.FindSecret(ctx, i.Client, instance.Namespace, fmt.Sprintf("%s/%s", constants.LabelNamespace, k.Name)); scr != nil {
				if instance.Status.Keys[index].SecretRef == nil ||
					instance.Status.Keys[index].SecretRef.Name != scr.Name ||
					instance.Status.Keys[index].SecretRef.Key != scr.Labels[fmt.Sprintf("%s/%s", constants.LabelNamespace, k.Name)] {
					return true
				}
			} else {
				return true
			}
		}
	}
	return false
}

func (i resolveKeysAction) Handle(ctx context.Context, instance *rhtasv1alpha1.Tuf) *action.Result {
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.Ready,
			Status: v1.ConditionFalse, Reason: constants.Pending, Message: "Resolving keys"})
	}

	if cap(instance.Status.Keys) < len(instance.Spec.Keys) {
		instance.Status.Keys = make([]rhtasv1alpha1.TufKey, 0, len(instance.Spec.Keys))
	}
	for index, key := range instance.Spec.Keys {
		k, err := i.handleKey(ctx, instance, &key)
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
		if len(instance.Status.Keys) < index+1 {
			instance.Status.Keys = append(instance.Status.Keys, *k)
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:   key.Name,
				Status: v1.ConditionTrue,
				Reason: constants.Ready,
			})
		} else {
			if !reflect.DeepEqual(*k, instance.Status.Keys[index]) {
				instance.Status.Keys[index] = *k
				meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
					Type:   key.Name,
					Status: v1.ConditionTrue,
					Reason: constants.Ready,
				})
			}
		}
		if index == len(instance.Status.Keys)-1 {
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.Ready,
				Status: v1.ConditionFalse, Reason: constants.Creating, Message: "Keys resolved"})
		}
	}
	return i.StatusUpdate(ctx, instance)
}

func (i resolveKeysAction) handleKey(ctx context.Context, instance *rhtasv1alpha1.Tuf, key *rhtasv1alpha1.TufKey) (*rhtasv1alpha1.TufKey, error) {
	switch {
	case key.SecretRef == nil:
		sks, err := i.discoverSecret(ctx, instance.Namespace, key)
		if err != nil {
			return nil, err
		}
		key.SecretRef = sks
		return key, nil
	case key.SecretRef != nil:
		return key, nil
	default:
		return nil, errors.New(fmt.Sprintf("Unable to resolve %s key. Enable autodiscovery or set secret reference.", key.Name))
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
			LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
				Name: s.Name,
			},
		}, nil
	}

	return nil, errors.New("secret not found")
}
