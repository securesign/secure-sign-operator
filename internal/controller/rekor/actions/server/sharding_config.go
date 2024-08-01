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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"
)

const (
	cmName             = "rekor-sharding-config-"
	shardingConfigName = "sharding-config.yaml"
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

	content, err := createShardingConfigData(instance.Spec.Sharding)
	if err != nil {
		i.Failed(fmt.Errorf("ShardingConfig: %w", err))
	}

	if instance.Status.ServerConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Failed(fmt.Errorf("ShardingConfig: %w", err))
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data, content) {
				return i.Continue()
			}
		}
	}

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

	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.ServerCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Creating,
		Message: "Sharding config created",
	})
	i.Recorder.Eventf(instance, v1.EventTypeNormal, "ShardingConfigCreated", "ConfigMap with sharding configuration created: %s", newConfig.Name)
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
