package actions

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"context"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const tufKeysSecretFormat = "tuf-keys-%s"

func NewResolveKeysAction() action.Action[*rhtasv1.Tuf] {
	return &resolveKeysAction{}
}

type resolveKeysAction struct {
	action.BaseAction
}

func (i resolveKeysAction) Name() string {
	return "resolve keys"
}

func (i resolveKeysAction) CanHandle(_ context.Context, instance *rhtasv1.Tuf) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Pending
}

func (i resolveKeysAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	autodiscoveredData := make(map[string][]byte)
	resolvedKeys := make([]rhtasv1.TufKeyStatus, 0, 4)

	for _, key := range trustroot.ActiveKeys(instance) {
		var (
			resolved  trustroot.Resolved
			err       error
			secretRef *rhtasv1.SecretKeySelector
		)
		if key == trustroot.Fulcio {
			binding := trustroot.FulcioBinding(instance)
			resolved, err = trustroot.ResolveFulcio(ctx, i.Client, instance.Namespace, binding)
			secretRef = binding.SecretRef
		} else {
			binding := trustroot.Binding(instance, key)
			resolved, err = trustroot.Resolve(ctx, i.Client, instance.Namespace, key, binding)
			secretRef = binding.SecretRef
		}
		if err != nil {
			if errors.Is(err, reconcile.TerminalError(nil)) {
				return i.Error(ctx, err, instance,
					v1.Condition{
						Type:    key.String(),
						Status:  v1.ConditionFalse,
						Reason:  state.Failure.String(),
						Message: err.Error()},
				)
			}
			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.ReadyCondition,
				Status: v1.ConditionFalse, Reason: state.Pending.String(), Message: "Resolving keys",
				ObservedGeneration: instance.Generation})

			meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
				Type:    key.String(),
				Status:  v1.ConditionFalse,
				Reason:  state.Failure.String(),
				Message: err.Error(),
			})
			if _, err := i.PersistStatus(ctx, instance); err != nil {
				return i.Error(ctx, err, instance)
			}
			return i.RequeueAfter(5 * time.Second)
		}

		if secretRef != nil {
			resolvedKeys = append(resolvedKeys, rhtasv1.TufKeyStatus{Name: key.String(), SecretRef: secretRef})
			continue
		}
		autodiscoveredData[key.String()] = resolved.Material
		resolvedKeys = append(resolvedKeys, rhtasv1.TufKeyStatus{Name: key.String()})
	}

	if len(autodiscoveredData) > 0 {
		secretName := fmt.Sprintf(tufKeysSecretFormat, instance.Name)
		secret := &corev1.Secret{
			ObjectMeta: v1.ObjectMeta{
				Name:      secretName,
				Namespace: instance.Namespace,
			},
		}
		componentLabels := labels.ForComponent(tufConstants.ComponentName, instance.Name)
		if _, err := k8sutils.CreateOrUpdate(ctx, i.Client, secret,
			ensure.ControllerReference[*corev1.Secret](instance, i.Client),
			ensure.Labels[*corev1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
			k8sutils.EnsureSecretData(false, autodiscoveredData),
		); err != nil {
			return i.Error(ctx, err, instance)
		}

		for idx := range resolvedKeys {
			if resolvedKeys[idx].SecretRef == nil {
				resolvedKeys[idx].SecretRef = &rhtasv1.SecretKeySelector{
					LocalObjectReference: rhtasv1.LocalObjectReference{Name: secretName},
					Key:                  resolvedKeys[idx].Name,
				}
			}
		}
	}

	changed := len(instance.Status.Keys) != len(resolvedKeys)
	if changed {
		instance.Status.Keys = make([]rhtasv1.TufKeyStatus, len(resolvedKeys))
	}
	for index, key := range resolvedKeys {
		if !reflect.DeepEqual(key, instance.Status.Keys[index]) {
			instance.Status.Keys[index] = key
			changed = true
		}
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{
			Type:   key.Name,
			Status: v1.ConditionTrue,
			Reason: state.Ready.String(),
		})
	}

	active := make(map[string]bool, len(resolvedKeys))
	for _, key := range resolvedKeys {
		active[key.Name] = true
	}
	for _, name := range []string{trustroot.Rekor.String(), trustroot.CTFE.String(), trustroot.Fulcio.String(), trustroot.TSA.String()} {
		if !active[name] && meta.RemoveStatusCondition(&instance.Status.Conditions, name) {
			changed = true
		}
	}

	if !changed {
		return i.Continue()
	}
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}
