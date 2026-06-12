package actions

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewResolveKeysAction() action.Action[*rhtasv1.Tuf] {
	return &resolveKeysAction{}
}

type resolveKeysAction struct {
	action.BaseAction
}

func (i resolveKeysAction) Name() string {
	return "resolve keys"
}

func (i resolveKeysAction) CanHandle(ctx context.Context, instance *rhtasv1.Tuf) bool {
	if state.FromInstance(instance, constants.ReadyCondition) < state.Pending {
		return false
	}
	return !tufKeysMatchStatus(instance.Spec.Keys, instance.Status.Keys)
}

func tufKeysMatchStatus(specKeys []rhtasv1.TufKey, statusKeys []rhtasv1.TufKeyStatus) bool {
	if len(specKeys) != len(statusKeys) {
		return false
	}
	for i := range specKeys {
		if !statusKeys[i].EqualsSpec(specKeys[i]) {
			return false
		}
	}
	return true
}

func (i resolveKeysAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	if state.FromInstance(instance, constants.ReadyCondition) != state.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.ReadyCondition,
			Status: v1.ConditionFalse, Reason: state.Pending.String(), Message: "Resolving keys",
			ObservedGeneration: instance.Generation})
	}

	if len(instance.Status.Keys) != len(instance.Spec.Keys) {
		instance.Status.Keys = make([]rhtasv1.TufKeyStatus, 0, len(instance.Spec.Keys))
	}
	for index, key := range instance.Spec.Keys {
		k, err := i.handleKey(ctx, instance, &key)
		if err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.ReadyCondition,
				Status: v1.ConditionFalse, Reason: state.Pending.String(), Message: "Resolving keys",
				ObservedGeneration: instance.Generation})

			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:    key.Name,
				Status:  v1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
			if _, err := i.PersistStatus(ctx, instance); err != nil {
				return i.Error(ctx, err, instance)
			}
			return i.RequeueAfter(5 * time.Second)
		}
		ks := rhtasv1.TufKeyStatus{Name: k.Name, SecretRef: k.SecretRef}
		if len(instance.Status.Keys) < index+1 {
			instance.Status.Keys = append(instance.Status.Keys, ks)
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:   key.Name,
				Status: v1.ConditionTrue,
				Reason: state.Ready.String(),
			})
		} else {
			if !reflect.DeepEqual(ks, instance.Status.Keys[index]) {
				instance.Status.Keys[index] = ks
				meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
					Type:   key.Name,
					Status: v1.ConditionTrue,
					Reason: state.Ready.String(),
				})
			}
		}
		if index == len(instance.Spec.Keys)-1 {
			if _, err := i.PersistStatus(ctx, instance); err != nil {
				return i.Error(ctx, err, instance)
			}
			return i.Continue()
		}
	}
	return i.Continue()
}

func (i resolveKeysAction) handleKey(ctx context.Context, instance *rhtasv1.Tuf, key *rhtasv1.TufKey) (*rhtasv1.TufKey, error) {
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
		return nil, fmt.Errorf("unable to resolve %s key. Enable autodiscovery or set secret reference", key.Name)
	}
}

func (i resolveKeysAction) discoverSecret(ctx context.Context, namespace string, key *rhtasv1.TufKey) (*rhtasv1.SecretKeySelector, error) {
	labelName := labels.LabelNamespace + "/" + key.Name
	s, err := k8sutils.FindSecret(ctx, i.Client, namespace, labelName)
	if err != nil {
		return nil, err
	}
	if s != nil {
		keySelector := s.Labels[labelName]
		if keySelector == "" {
			err = fmt.Errorf("label %s is empty", labelName)
			return nil, err
		}
		return &rhtasv1.SecretKeySelector{
			Key: keySelector,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: s.Name,
			},
		}, nil
	}

	return nil, errors.New("secret not found")
}
