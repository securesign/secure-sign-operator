package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/resolvePubKey"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	tufKeysSecretFormat = "tuf-keys-%s"

	rekorKeyName  = "rekor.pub"
	ctfeKeyName   = "ctfe.pub"
	fulcioKeyName = "fulcio_v1.crt.pem"
	tsaKeyName    = "tsa.certchain.pem"
)

var (
	ErrNoReadyComponent      = errors.New("no ready component instance found")
	ErrTrustMaterialNotReady = errors.New("trust material not yet available")
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
	return !instance.Status.MatchesKeys(instance.Spec.Keys)
}

func (i resolveKeysAction) Handle(ctx context.Context, instance *rhtasv1.Tuf) *action.Result {
	if state.FromInstance(instance, constants.ReadyCondition) != state.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, v1.Condition{Type: constants.ReadyCondition,
			Status: v1.ConditionFalse, Reason: state.Pending.String(), Message: "Resolving keys",
			ObservedGeneration: instance.Generation})
	}

	autodiscoveredData := make(map[string][]byte)
	resolvedKeys := make([]rhtasv1.TufKey, 0, len(instance.Spec.Keys))

	for _, key := range instance.Spec.Keys {
		if key.SecretRef != nil {
			resolvedKeys = append(resolvedKeys, key)
			continue
		}

		trustMaterial, err := discoverFromStatus(ctx, i.Client, instance.Namespace, key.Name)
		if err != nil {
			if errors.Is(err, reconcile.TerminalError(nil)) {
				return i.Error(ctx, err, instance)
			}
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

		autodiscoveredData[key.Name] = []byte(trustMaterial)
		resolvedKeys = append(resolvedKeys, rhtasv1.TufKey{Name: key.Name})
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

	if len(instance.Status.Keys) != len(resolvedKeys) {
		instance.Status.Keys = make([]rhtasv1.TufKeyStatus, 0, len(resolvedKeys))
	}
	for index, key := range resolvedKeys {
		ks := rhtasv1.TufKeyStatus(key)
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
	}

	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

type trustMaterialSource struct {
	list             client.ObjectList
	getTrustMaterial func(runtime.Object) string
}

var keyToSource = map[string]trustMaterialSource{
	rekorKeyName:  {list: &rhtasv1.RekorList{}, getTrustMaterial: func(obj runtime.Object) string { return obj.(*rhtasv1.Rekor).Status.PublicKey }},
	ctfeKeyName:   {list: &rhtasv1.CTlogList{}, getTrustMaterial: func(obj runtime.Object) string { return obj.(*rhtasv1.CTlog).Status.PublicKey }},
	fulcioKeyName: {list: &rhtasv1.FulcioList{}, getTrustMaterial: func(obj runtime.Object) string { return obj.(*rhtasv1.Fulcio).Status.CertificateChain }},
	tsaKeyName:    {list: &rhtasv1.TimestampAuthorityList{}, getTrustMaterial: func(obj runtime.Object) string { return obj.(*rhtasv1.TimestampAuthority).Status.CertificateChain }},
}

func discoverFromStatus(ctx context.Context, cli client.Client, namespace, keyName string) (string, error) {
	src, ok := keyToSource[keyName]
	if !ok {
		return "", reconcile.TerminalError(fmt.Errorf("unknown key %s — no autodiscovery mapping defined", keyName))
	}

	list := src.list.DeepCopyObject().(client.ObjectList)
	if err := cli.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return "", fmt.Errorf("listing component instances: %w", err)
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		return "", reconcile.TerminalError(err)
	}

	for _, item := range items {
		condAware, ok := item.(apis.ConditionsAwareObject)
		if !ok {
			continue
		}
		if !meta.IsStatusConditionTrue(condAware.GetConditions(), constants.ReadyCondition) {
			continue
		}
		material := src.getTrustMaterial(item)
		if err := resolvePubKey.ValidatePEM([]byte(material)); err != nil {
			return "", fmt.Errorf("%w: %w", ErrTrustMaterialNotReady, err)
		}
		return material, nil
	}
	return "", ErrNoReadyComponent
}
