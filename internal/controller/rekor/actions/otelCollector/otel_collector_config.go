package otelcollector

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes/ensure"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

const (
	cmName               = "otel-collector-config-"
	collectorConfigLabel = "otel-collector-conf"
	collectorConfigName  = "otel-collector-config.yaml"
)

func NewCollectorConfigAction() action.Action[*rhtasv1alpha1.Rekor] {
	return &collectorConfig{}
}

type collectorConfig struct {
	action.BaseAction
}

func (i collectorConfig) Name() string {
	return "collector config"
}

func (i collectorConfig) CanHandle(_ context.Context, instance *rhtasv1alpha1.Rekor) bool {
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

func (i collectorConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.Rekor) *action.Result {
	labels := labels.ForResource(actions.OtelCollectorComponentName, actions.OtelCollectorDeploymentName, instance.Name, collectorConfigLabel)

	content, err := createCollectorConfigData()
	if err != nil {
		i.Error(ctx, reconcile.TerminalError(fmt.Errorf("could not create collector config: %w", err)), instance)
	}

	// verify existing config
	if instance.Status.OtelCollectorConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.OtelCollectorConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Error(ctx, fmt.Errorf("can't get CollectorConfig: %w", err), instance)
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data, content) {
				return i.Continue()
			} else {
				i.Logger.Info("Remove invalid ConfigMap with collector configuration", "Name", cfg.Name)
				err = i.Client.Delete(ctx, cfg)
				if err != nil {
					i.Logger.Error(err, "Failed to remove ConfigMap", "name", cfg.Name)
				}
			}
		}
	}
	// invalidate
	instance.Status.OtelCollectorConfigRef = nil

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
		return i.Error(ctx, fmt.Errorf("could not create collector config: %w", err), instance)
	}

	i.Recorder.Eventf(instance, v1.EventTypeNormal, "CollectorConfigCreated", "ConfigMap with collector configuration created: %s", newConfig.Name)
	instance.Status.OtelCollectorConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    actions.OtelCollectorCondition,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Creating,
		Message: "Collector config created",
	})

	result := i.StatusUpdate(ctx, instance)
	if action.IsSuccess(result) {
		i.cleanup(ctx, instance, labels)
	}
	return result
}

func (i collectorConfig) cleanup(ctx context.Context, instance *rhtasv1alpha1.Rekor, configLabels map[string]string) {
	if instance.Status.OtelCollectorConfigRef == nil || instance.Status.OtelCollectorConfigRef.Name == "" {
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
		if partialConfig.Name == instance.Status.OtelCollectorConfigRef.Name {
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
			i.Recorder.Eventf(instance, v1.EventTypeWarning, "CollectorConfigDeleted", "Unable to delete secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid ConfigMap with otel-collector configuration", "name", partialConfig.Name)
		i.Recorder.Eventf(instance, v1.EventTypeNormal, "CollectorConfigDeleted", "ConfigMap with collector configuration deleted: %s", partialConfig.Name)
	}
}

func createCollectorConfigData() (map[string]string, error) {
	config := map[string]any{
		"receivers": map[string]any{
			"otlp": map[string]any{
				"protocols": map[string]any{
					"grpc": map[string]any{"endpoint": "0.0.0.0:4317"},
					"http": map[string]any{"endpoint": "0.0.0.0:4318"},
				},
			},
		},
		"exporters": map[string]any{
			"prometheus": map[string]any{
				"endpoint": "0.0.0.0:9464",
			},
			"debug": map[string]any{
				"verbosity": "detailed",
			},
		},
		"service": map[string]any{
			"pipelines": map[string]any{
				"metrics": map[string]any{
					"receivers": []string{"otlp"},
					"exporters": []string{"prometheus", "debug"},
				},
			},
		},
	}

	yamlData, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	return map[string]string{collectorConfigName: string(yamlData)}, nil
}
