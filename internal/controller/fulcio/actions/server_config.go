package actions

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/controller/traits"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

const (
	configResourceLabel = "server-config"
	serverConfigName    = "config.yaml"
)

func NewServerConfigAction() action.Action[*rhtasv1.Fulcio] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "create server config"
}

func (i serverConfig) CanHandle(ctx context.Context, instance *rhtasv1.Fulcio) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1.Fulcio) *action.Result {
	var (
		err error
	)
	configLabel := labels.ForResource(ComponentName, DeploymentName, instance.Name, configResourceLabel)

	config, err := yaml.Marshal(utils.NewFulcioServerConfig(instance.Spec.Config))
	if err != nil {
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not marshal fulcio config: %w", err)), instance)
	}

	// verify existing config
	if instance.Status.ServerConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Error(ctx, fmt.Errorf("can't get FulcioConfig: %w", err), instance)
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data[serverConfigName], string(config)) {
				return i.Continue()
			} else {
				i.Logger.Info("Remove invalid ConfigMap with fulcio-server configuration", "name", cfg.Name)
				err = i.Client.Delete(ctx, cfg)
				if err != nil {
					i.Logger.Error(err, "Failed to remove ConfigMap", "name", cfg.Name)
				}
			}
		}
	}
	// invalidate
	instance.Status.ServerConfigRef = nil

	desiredConfig, err := traits.Apply(
		corev1ac.ConfigMap("", instance.Namespace).
			WithGenerateName("fulcio-config-").
			WithImmutable(true).
			WithData(map[string]string{serverConfigName: string(config)}),
		traits.ControllerReference[*corev1ac.ConfigMapApplyConfiguration](instance, i.Client),
		traits.Labels[*corev1ac.ConfigMapApplyConfiguration](configLabel),
	)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not build server config: %w", err), instance)
	}

	newConfig, err := kubernetes.Create(ctx, i.Client, desiredConfig)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Server config: %w", err), instance)
	}

	i.Recorder.Eventf(instance, newConfig, v1.EventTypeNormal, "FulcioConfigUpdated", "Updated", "Fulcio config updated: %s", newConfig.GetName())
	instance.Status.ServerConfigRef = &rhtasv1.LocalObjectReference{Name: newConfig.GetName()}

	meta.SetStatusCondition(&instance.Status.Conditions,
		metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "Server config created",
			ObservedGeneration: instance.Generation},
	)

	changed, err := i.PersistStatus(ctx, instance)
	if err != nil {
		return i.Error(ctx, err, instance)
	}
	i.cleanup(ctx, instance, configLabel)
	if changed {
		return i.Return()
	}
	return i.Continue()
}

func (i serverConfig) cleanup(ctx context.Context, instance *rhtasv1.Fulcio, configLabels map[string]string) {
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
			i.Recorder.Eventf(instance, nil, v1.EventTypeWarning, "FulcioConfigDeleted", "CleanupFailed", "Unable to delete secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid ConfigMap with Fulcio configuration", "name", partialConfig.Name)
		i.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "FulcioConfigDeleted", "Deleted", "Fulcio config deleted: %s", partialConfig.Name)
	}
}
