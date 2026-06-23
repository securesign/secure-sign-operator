package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	yaml "sigs.k8s.io/yaml/goyaml.v2"
)

const (
	ntpConfigLabel = "ntp-monitoring-conf"
	ntpConfigName  = "ntp-config.yaml"
)

type ntpMonitoringAction struct {
	action.BaseAction
}

func NewNtpMonitoringAction() action.Action[*rhtasv1.TimestampAuthority] {
	return &ntpMonitoringAction{}
}

func (i ntpMonitoringAction) Name() string {
	return "ntpMonitoring"
}

func (i ntpMonitoringAction) CanHandle(_ context.Context, instance *rhtasv1.TimestampAuthority) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Creating
}

func (i ntpMonitoringAction) Handle(ctx context.Context, instance *rhtasv1.TimestampAuthority) *action.Result {
	// No operator-managed ConfigMap needed — just sync status.
	if instance.Spec.NTPMonitoring.Config == nil || instance.Spec.NTPMonitoring.Config.NtpConfigRef != nil {
		newRef := ntpConfigRefFromSpec(instance)
		if reflect.DeepEqual(newRef, instance.Status.NtpConfigRef) {
			return i.Continue()
		}
		instance.Status.NtpConfigRef = newRef
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "NTP monitoring configured",
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	ntpConfig, err := i.marshalNTPMonitoringConfig(instance.Spec.NTPMonitoring.Config)
	if err != nil {
		return i.Error(ctx, err, instance, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
	}

	// verify existing config
	if instance.Status.NtpConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.NtpConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Error(ctx, fmt.Errorf("NTPConfig: %w", err), instance)
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data[ntpConfigName], string(ntpConfig)) {
				return i.Continue()
			}
			i.Logger.Info("Config data changed, existing config will be replaced", "Name", cfg.Name)
		}
	}

	configLabel := labels.ForResource(ComponentName, DeploymentName, instance.Name, ntpConfigLabel)

	// create new config
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: NtpCMName,
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		configMap,
		ensure.ControllerReference[*v1.ConfigMap](instance, i.Client),
		ensure.Labels[*v1.ConfigMap](slices.Collect(maps.Keys(configLabel)), configLabel),
		kubernetes.EnsureConfigMapData(true, map[string]string{ntpConfigName: string(ntpConfig)}),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create ntp config: %w", err), instance)
	}

	i.Recorder.Eventf(instance, configMap, v1.EventTypeNormal, "NTPConfigUpdated", "Updated", "NTP config updated: %s", configMap.Name)
	instance.Status.NtpConfigRef = &rhtasv1.LocalObjectReference{Name: configMap.Name}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               constants.ReadyCondition,
		Status:             metav1.ConditionFalse,
		Reason:             state.Creating.String(),
		Message:            "NTP monitoring configured",
		ObservedGeneration: instance.Generation,
	})

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

func (i ntpMonitoringAction) marshalNTPMonitoringConfig(instance *rhtasv1.NtpMonitoringConfig) ([]byte, error) {
	if instance == nil {
		return make([]byte, 0), nil
	}

	ntpConfig := tsaUtils.NtpConfig{
		RequestAttempts: instance.RequestAttempts,
		RequestTimeout:  instance.RequestTimeout,
		NumServers:      instance.NumServers,
		MaxTimeDelta:    instance.MaxTimeDelta,
		ServerThreshold: instance.ServerThreshold,
		Period:          instance.Period,
		Servers:         instance.Servers,
	}
	config, err := yaml.Marshal(&ntpConfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (i ntpMonitoringAction) cleanup(ctx context.Context, instance *rhtasv1.TimestampAuthority, configLabels map[string]string) {
	if instance.Status.NtpConfigRef == nil || instance.Status.NtpConfigRef.Name == "" {
		i.Logger.Error(errors.New("new ConfigMap name is empty"), "unable to clean old objects", "namespace", instance.Namespace)
		return
	}

	partialConfigs, err := kubernetes.ListConfigMaps(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(configLabels).String())
	if err != nil {
		i.Logger.Error(err, "problem with finding configmap")
		return
	}
	for _, cm := range partialConfigs.Items {
		if cm.Name == instance.Status.NtpConfigRef.Name {
			continue
		}
		err = i.Client.Delete(ctx, &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cm.Name,
				Namespace: cm.Namespace,
			},
		})
		if err != nil {
			i.Logger.Error(err, "problem with deleting configmap", "name", cm.Name)
			i.Recorder.Eventf(instance, nil, v1.EventTypeWarning, "NTPConfigDeleted", "CleanupFailed", "Unable to delete configmap: %s", cm.Name)
			continue
		}
		i.Logger.Info("Remove old ConfigMap with NTP configuration", "name", cm.Name)
		i.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "NTPConfigDeleted", "Deleted", "NTP config deleted: %s", cm.Name)
	}
}

func ntpConfigRefFromSpec(instance *rhtasv1.TimestampAuthority) *rhtasv1.LocalObjectReference {
	if instance.Spec.NTPMonitoring.Config != nil && instance.Spec.NTPMonitoring.Config.NtpConfigRef != nil {
		return &rhtasv1.LocalObjectReference{Name: instance.Spec.NTPMonitoring.Config.NtpConfigRef.Name}
	}
	return nil
}
