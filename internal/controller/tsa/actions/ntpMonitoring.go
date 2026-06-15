package actions

import (
	"context"
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
	cmName         = "ntp-monitoring-config-"
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

func (i ntpMonitoringAction) CanHandle(ctx context.Context, instance *rhtasv1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.ReadyCondition)

	switch {
	case c == nil:
		return false
	case state.FromCondition(c) < state.Creating:
		return false
	case !instance.Spec.NTPMonitoring.Enabled:
		return false
	case instance.Status.NTPMonitoring == nil:
		return true
	default:
		return !instance.Status.NTPMonitoring.MatchesSpec(instance.Spec.NTPMonitoring)
	}
}

func (i ntpMonitoringAction) Handle(ctx context.Context, instance *rhtasv1.TimestampAuthority) *action.Result {
	// No inline config to generate — either no config at all or user-provided ConfigMap ref.
	if instance.Spec.NTPMonitoring.Config == nil || instance.Spec.NTPMonitoring.Config.NtpConfigRef != nil {
		instance.Status.NTPMonitoring = i.buildNTPStatus(&instance.Spec.NTPMonitoring, "")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            "NTP monitoring configured", //nolint:goconst
			ObservedGeneration: instance.Generation,
		})
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	// From here: inline config fields need to be generated into a ConfigMap.

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

	l := labels.For(ComponentName, DeploymentName, instance.Name)
	l[labels.LabelResource] = ntpConfigLabel

	if instance.Status.NTPMonitoring != nil && instance.Status.NTPMonitoring.NtpConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.NTPMonitoring.NtpConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Error(ctx, fmt.Errorf("NTPConfig: %w", err), instance)
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data[ntpConfigName], string(ntpConfig)) {
				return i.Continue()
			} else {
				i.Logger.Info("Remove invalid ConfigMap with NTP configuration", "Name", cfg.Name)
				_ = i.Client.Delete(ctx, cfg)
			}
		}
	}

	var resolvedCMName string

	partialConfigs, err := kubernetes.ListConfigMaps(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(l).String())
	if err != nil {
		i.Logger.Error(err, "problem with finding configmap", "namespace", instance.Namespace)
	}
	for _, partialSecret := range partialConfigs.Items {
		cm, err := kubernetes.GetConfigMap(ctx, i.Client, partialSecret.Namespace, partialSecret.Name)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("can't load configMap data %w", err), instance)
		}
		if reflect.DeepEqual(cm.Data[ntpConfigName], string(ntpConfig)) && resolvedCMName == "" {
			i.Recorder.Eventf(instance, nil, v1.EventTypeNormal, "NTPConfigDiscovered", "Discovered", "Existing ConfigMap with NTP configuration discovered: %s", cm.Name)
			resolvedCMName = cm.Name
			instance.Status.NTPMonitoring = i.buildNTPStatus(&instance.Spec.NTPMonitoring, resolvedCMName)
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               constants.ReadyCondition,
				Status:             metav1.ConditionFalse,
				Reason:             state.Creating.String(),
				Message:            "NTP config discovered",
				ObservedGeneration: instance.Generation,
			})
		} else {
			i.Logger.Info("Remove invalid ConfigMap with NTP configuration", "Name", cm.Name)
			_ = i.Client.Delete(ctx, cm)
		}
	}
	if resolvedCMName != "" {
		return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
	}

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: NtpCMName,
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		configMap,
		ensure.ControllerReference[*v1.ConfigMap](instance, i.Client),
		ensure.Labels[*v1.ConfigMap](slices.Collect(maps.Keys(l)), l),
		kubernetes.EnsureConfigMapData(
			true, map[string]string{ntpConfigName: string(ntpConfig)}),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create ntp config: %w", err), instance)
	}

	instance.Status.NTPMonitoring = i.buildNTPStatus(&instance.Spec.NTPMonitoring, configMap.Name)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               constants.ReadyCondition,
		Status:             metav1.ConditionFalse,
		Reason:             state.Creating.String(),
		Message:            "NTP monitoring configured",
		ObservedGeneration: instance.Generation,
	})
	return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
}

func (i ntpMonitoringAction) marshalNTPMonitoringConfig(instance *rhtasv1.NtpMonitoringConfig) ([]byte, error) {
	var (
		err    error
		config []byte
	)

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
	config, err = yaml.Marshal(&ntpConfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (i ntpMonitoringAction) buildNTPStatus(spec *rhtasv1.NTPMonitoring, cmName string) *rhtasv1.NTPMonitoringStatus {
	status := &rhtasv1.NTPMonitoringStatus{}
	if spec.Config == nil {
		return status
	}
	if spec.Config.NtpConfigRef != nil {
		status.NtpConfigRef = spec.Config.NtpConfigRef
	} else if cmName != "" {
		status.NtpConfigRef = &rhtasv1.LocalObjectReference{Name: cmName}
	}
	return status
}
