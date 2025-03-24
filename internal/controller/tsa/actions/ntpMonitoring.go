package actions

import (
	"context"
	"fmt"
	"reflect"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	cmName         = "ntp-monitoring-config-"
	ntpConfigLabel = "ntp-monitoring-conf"
	ntpConfigName  = "ntp-config.yaml"
)

type ntpMonitoringAction struct {
	action.BaseAction
}

func NewNtpMonitoringAction() action.Action[*rhtasv1alpha1.TimestampAuthority] {
	return &ntpMonitoringAction{}
}

func (i ntpMonitoringAction) Name() string {
	return "ntpMonitoring"
}

func (i ntpMonitoringAction) CanHandle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)

	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	case instance.Spec.NTPMonitoring.Config != nil:
		return !equality.Semantic.DeepEqual(instance.Spec.NTPMonitoring, instance.Status.NTPMonitoring)
	case c.Reason == constants.Ready:
		return instance.Generation != c.ObservedGeneration
	default:
		return false
	}
}

func (i ntpMonitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {

	var newStatus *rhtasv1alpha1.NTPMonitoring
	if instance.Status.NTPMonitoring == nil {
		newStatus = instance.Spec.NTPMonitoring.DeepCopy()
	} else {
		newStatus = instance.Status.NTPMonitoring
	}

	if instance.Spec.NTPMonitoring.Config.NtpConfigRef != nil {
		i.alignStatusFields(instance, newStatus, cmName)
		instance.Status.NTPMonitoring = newStatus
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "NTP monitoring configured",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	}

	var (
		err    error
		cmName string
	)

	ntpConfig, err := i.handleNTPMonitoring(instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	l := labels.For(ComponentName, DeploymentName, instance.Name)
	l[labels.LabelResource] = ntpConfigLabel

	if newStatus.Config.NtpConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, newStatus.Config.NtpConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Failed(fmt.Errorf("NTPConfig: %w", err))
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
	newStatus.Config.NtpConfigRef = nil

	partialConfigs, err := kubernetes.ListConfigMaps(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(l).String())
	if err != nil {
		i.Logger.Error(err, "problem with finding configmap", "namespace", instance.Namespace)
	}
	for _, partialSecret := range partialConfigs.Items {
		cm, err := kubernetes.GetConfigMap(ctx, i.Client, partialSecret.Namespace, partialSecret.Name)
		if err != nil {
			return i.Failed(fmt.Errorf("can't load configMap data %w", err))
		}
		if reflect.DeepEqual(cm.Data[ntpConfigName], string(ntpConfig)) && newStatus.Config.NtpConfigRef == nil {
			i.Recorder.Eventf(instance, v1.EventTypeNormal, "NTPConfigDiscovered", "Existing ConfigMap with NTP configuration discovered: %s", cm.Name)
			i.alignStatusFields(instance, newStatus, cm.Name)
			instance.Status.NTPMonitoring = newStatus
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               constants.Ready,
				Status:             metav1.ConditionFalse,
				Reason:             constants.Creating,
				Message:            "NTP config discovered",
				ObservedGeneration: instance.Generation,
			})
		} else {
			i.Logger.Info("Remove invalid ConfigMap with NTP configuration", "Name", cm.Name)
			_ = i.Client.Delete(ctx, cm)
		}
	}
	if newStatus.Config.NtpConfigRef != nil {
		return i.StatusUpdate(ctx, instance)
	}

	configMap := kubernetes.CreateImmutableConfigmap(NtpCMName, instance.Namespace, l, map[string]string{ntpConfigName: string(ntpConfig)})
	if err = controllerutil.SetControllerReference(instance, configMap, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for ConfigMap: %w", err))
	}
	_, err = i.Ensure(ctx, configMap)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}
	cmName = configMap.Name

	i.alignStatusFields(instance, newStatus, cmName)
	instance.Status.NTPMonitoring = newStatus
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               constants.Ready,
		Status:             metav1.ConditionFalse,
		Reason:             constants.Creating,
		Message:            "NTP monitoring configured",
		ObservedGeneration: instance.Generation,
	})
	return i.StatusUpdate(ctx, instance)
}

func (i ntpMonitoringAction) handleNTPMonitoring(instance *rhtasv1alpha1.TimestampAuthority) ([]byte, error) {
	var (
		err    error
		config []byte
	)

	ntpConfig := tsaUtils.NtpConfig{
		RequestAttempts: instance.Spec.NTPMonitoring.Config.RequestAttempts,
		RequestTimeout:  instance.Spec.NTPMonitoring.Config.RequestTimeout,
		NumServers:      instance.Spec.NTPMonitoring.Config.NumServers,
		MaxTimeDelta:    instance.Spec.NTPMonitoring.Config.MaxTimeDelta,
		ServerThreshold: instance.Spec.NTPMonitoring.Config.ServerThreshold,
		Period:          instance.Spec.NTPMonitoring.Config.Period,
		Servers:         instance.Spec.NTPMonitoring.Config.Servers,
	}
	config, err = yaml.Marshal(&ntpConfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func (i ntpMonitoringAction) alignStatusFields(instance *rhtasv1alpha1.TimestampAuthority, newStatus *rhtasv1alpha1.NTPMonitoring, cmName string) {
	if newStatus == nil {
		newStatus = new(rhtasv1alpha1.NTPMonitoring)
	}
	instance.Spec.NTPMonitoring.DeepCopyInto(newStatus)

	if instance.Spec.NTPMonitoring.Config.NtpConfigRef != nil {
		newStatus.Config.NtpConfigRef = instance.Spec.NTPMonitoring.Config.NtpConfigRef
	} else if cmName != "" {
		newStatus.Config.NtpConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: cmName}
	}
}
