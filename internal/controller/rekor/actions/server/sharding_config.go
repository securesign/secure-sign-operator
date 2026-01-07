package server

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

const (
	cmName              = "rekor-sharding-config-"
	shardingConfigLabel = "rekor-sharding-conf"
	shardingConfigName  = "sharding-config.yaml"
)

func NewShardingConfigAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &shardingConfig{}
}

type shardingConfig struct {
	action.BaseAction
}

func (i shardingConfig) Name() string {
	return "sharding config"
}

func (i shardingConfig) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
	return state.FromInstance(instance, actions.ServerCondition) >= state.Creating
}

func (i shardingConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	labels := labels.ForResource(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name, shardingConfigLabel)

	content, err := createShardingConfigData(instance.Spec.Sharding)
	if err != nil {
		i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create sharding config: %w", err)), instance)
	}

	// verify existing config
	if instance.Status.ServerConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Error(ctx, fmt.Errorf("can't get ShardingConfig: %w", err), instance)
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data, content) {
				return i.Continue()
			} else {
				i.Logger.Info("Remove invalid ConfigMap with rekor-server configuration", "Name", cfg.Name)
				err = i.Client.Delete(ctx, cfg)
				if err != nil {
					i.Logger.Error(err, "Failed to remove ConfigMap", "name", cfg.Name)
				}
			}
		}
	}
	// invalidate
	instance.Status.ServerConfigRef = nil

	// create new config
	newConfig := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cmName,
			Namespace:    instance.Namespace,
		},
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		newConfig,
		ensure.ControllerReference[*v1.ConfigMap](instance, i.Client),
		ensure.Labels[*v1.ConfigMap](slices.Collect(maps.Keys(labels)), labels),
		kubernetes.EnsureConfigMapData(true, content),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create sharding config: %w", err), instance)
	}

	i.Recorder.Eventf(instance, v1.EventTypeNormal, "ShardingConfigCreated", "ConfigMap with sharding configuration created: %s", newConfig.Name)
	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  state.Creating.String(),
		Message: "Sharding config created",
	})

	result := i.StatusUpdate(ctx, instance)
	if action.IsSuccess(result) {
		i.cleanup(ctx, instance, labels)
	}
	return result
}

func (i shardingConfig) cleanup(ctx context.Context, instance *rhtasv1alpha1.Rekor, configLabels map[string]string) {
	if instance.Status.ServerConfigRef == nil || instance.Status.ServerConfigRef.Name == "" {
		i.Logger.Error(errors.New("new ConfigMap name is empty"), "unable to clean old objects", "namespace", instance.Namespace)
		return
	}

	// remove old server configmaps
	partialConfigs, err := kubernetes.ListConfigMaps(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(configLabels).String())
	if err != nil {
		i.Logger.Error(err, "problem with finding configmap")
		return
	}
	for _, partialConfig := range partialConfigs.Items {
		if partialConfig.Name == instance.Status.ServerConfigRef.Name {
			continue
		}

		err = i.Client.Delete(ctx, &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      partialConfig.Name,
				Namespace: partialConfig.Namespace,
			},
		})
		if err != nil {
			i.Logger.Error(err, "problem with deleting configmap", "name", partialConfig.Name)
			i.Recorder.Eventf(instance, v1.EventTypeWarning, "ShardingConfigDeleted", "Unable to delete secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid ConfigMap with rekor-sharding configuration", "name", partialConfig.Name)
		i.Recorder.Eventf(instance, v1.EventTypeNormal, "ShardingConfigDeleted", "ConfigMap with sharding configuration deleted: %s", partialConfig.Name)
	}
}

func createShardingConfigData(sharding []rhtasv1alpha1.RekorLogRange) (map[string]string, error) {
	var content string
	if len(sharding) > 0 {
		marshal, err := yaml.Marshal(sharding)
		if err != nil {
			return nil, err
		}
		content = string(marshal[:])
	}
	return map[string]string{shardingConfigName: content}, nil
}
