package actions

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/labels"
	cryptoutil "github.com/securesign/operator/internal/utils/crypto"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewResolveKeysAction() action.Action[*rhtasv1alpha1.Tuf] {
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

	return !equality.Semantic.DeepDerivative(instance.Spec.Keys, instance.Status.Keys)
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
		if index == len(instance.Spec.Keys)-1 {
			return i.Continue()
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
	case key.SecretRef != nil:
		// continue to validation
	default:
		return nil, fmt.Errorf("unable to resolve %s key. Enable autodiscovery or set secret reference", key.Name)
	}

	if cryptoutil.FIPSEnabled {
		if err := validateKey(i.Client, instance.Namespace, key); err != nil {
			return nil, fmt.Errorf("key %s is not FIPS-compliant: %w", key.Name, err)
		}
	}

	return key, nil
}

func (i resolveKeysAction) discoverSecret(ctx context.Context, namespace string, key *rhtasv1alpha1.TufKey) (*rhtasv1alpha1.SecretKeySelector, error) {
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
		return &rhtasv1alpha1.SecretKeySelector{
			Key: keySelector,
			LocalObjectReference: rhtasv1alpha1.LocalObjectReference{
				Name: s.Name,
			},
		}, nil
	}

	return nil, errors.New("secret not found")
}

func validateKey(cli client.Client, namespace string, key *rhtasv1alpha1.TufKey) error {
	if key.SecretRef == nil {
		return errors.New("secret reference is not set")
	}

	data, err := k8sutils.GetSecretData(cli, namespace, key.SecretRef)
	if err != nil {
		return err
	}

	switch key.Name {
	case "rekor.pub", "ctfe.pub":
		return cryptoutil.ValidatePublicKeyPEM(data)
	case "fulcio_v1.crt.pem", "tsa.certchain.pem":
		return cryptoutil.ValidateCertificatePEM(data)
	default:
		return fmt.Errorf("unsupported key %q", key.Name)
	}
}
