package server

import (
	"context"
	"fmt"
	"reflect"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	c := meta.FindStatusCondition(instance.Status.Conditions, actions.ServerCondition)
	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	default:
		return true
	}
}

func (i shardingConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)
	labels[constants.LabelResource] = shardingConfigLabel

	content, err := createShardingConfigData(instance.Spec.Sharding)
	if err != nil {
		i.Failed(fmt.Errorf("ShardingConfig: %w", err))
	}

	// verify existing config
	if instance.Status.ServerConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Failed(fmt.Errorf("ShardingConfig: %w", err))
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data, content) {
				return i.Continue()
			} else {
				i.Logger.Info("Remove invalid ConfigMap with rekor-server configuration", "Name", cfg.Name)
				_ = i.Client.Delete(ctx, cfg)
			}
		}
	}
	// invalidate
	instance.Status.ServerConfigRef = nil

	// try to discover existing config
	partialConfigs, err := kubernetes.ListConfigMaps(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(labels).String())
	if err != nil {
		i.Logger.Error(err, "problem with finding configmap", "namespace", instance.Namespace)
	}
	for _, partialSecret := range partialConfigs.Items {
		cm, err := kubernetes.GetConfigMap(ctx, i.Client, partialSecret.Namespace, partialSecret.Name)
		if err != nil {
			return i.Failed(fmt.Errorf("can't load configMap data %w", err))
		}
		if reflect.DeepEqual(cm.Data, content) && instance.Status.ServerConfigRef == nil {
			i.Recorder.Eventf(instance, v1.EventTypeNormal, "ShardingConfigDiscovered", "Existing ConfigMap with sharding configuration discovered: %s", cm.Name)
			instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: cm.Name}
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    actions.ServerCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Creating,
				Message: "Sharding config discovered",
			})
		} else {
			i.Logger.Info("Remove invalid ConfigMap with rekor-server configuration", "Name", cm.Name)
			_ = i.Client.Delete(ctx, cm)
		}
	}
	if instance.Status.ServerConfigRef != nil {
		return i.StatusUpdate(ctx, instance)
	}

	// create new config
	newConfig := kubernetes.CreateImmutableConfigmap(cmName, instance.Namespace, labels, content)
	if err = controllerutil.SetControllerReference(instance, newConfig, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("ShardingConfig: could not set controller reference for ConfigMap: %w", err))
	}

	_, err = i.Ensure(ctx, newConfig)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}
	i.Recorder.Eventf(instance, v1.EventTypeNormal, "ShardingConfigCreated", "ConfigMap with sharding configuration created: %s", newConfig.Name)
	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Creating,
		Message: "Sharding config created",
	})
	return i.StatusUpdate(ctx, instance)
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
