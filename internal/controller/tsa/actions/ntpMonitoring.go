package actions

import (
	"context"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	return (c.Reason == constants.Creating || c.Reason == constants.Ready) && instance.Spec.NTPMonitoring.Enabled && instance.Spec.NTPMonitoring.Config != nil && (instance.Status.NTPMonitoring == nil || !equality.Semantic.DeepDerivative(instance.Spec.NTPMonitoring, *instance.Status.NTPMonitoring))
}

func (i ntpMonitoringAction) Handle(ctx context.Context, instance *rhtasv1alpha1.TimestampAuthority) *action.Result {
	var (
		err    error
		cmName string
	)

	ntpConfig, err := i.handleNTPMonitoring(instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	if ntpConfig != nil {
		labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
		configMap := kubernetes.CreateConfigMap(NtpCMName, instance.Namespace, labels, map[string]string{"ntp-config.yaml": string(ntpConfig)})
		if err = controllerutil.SetControllerReference(instance, configMap, i.Client.Scheme()); err != nil {
			return i.Failed(fmt.Errorf("could not set controller reference for ConfigMap: %w", err))
		}

		if err = i.Client.DeleteAllOf(ctx, &v1.ConfigMap{}, client.InNamespace(instance.Namespace), client.MatchingLabels(labels)); err != nil {
			return i.Failed(err)
		}

		_, err = i.Ensure(ctx, configMap)
		if err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			return i.FailedWithStatusUpdate(ctx, err, instance)
		}
		cmName = configMap.Name
	}

	if instance.Status.NTPMonitoring == nil {
		instance.Status.NTPMonitoring = new(v1alpha1.NTPMonitoring)
	}
	instance.Spec.NTPMonitoring.DeepCopyInto(instance.Status.NTPMonitoring)

	if instance.Spec.NTPMonitoring.Config.NtpConfigRef == nil {
		instance.Status.NTPMonitoring.Config.NtpConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: cmName}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    constants.Ready,
		Status:  metav1.ConditionFalse,
		Reason:  constants.Creating,
		Message: "NTP monitoring configured"},
	)
	return i.StatusUpdate(ctx, instance)
}

func (i ntpMonitoringAction) handleNTPMonitoring(instance *rhtasv1alpha1.TimestampAuthority) ([]byte, error) {
	var (
		err    error
		config []byte
	)

	if instance.Spec.NTPMonitoring.Config.NtpConfigRef != nil {
		return nil, nil
	}

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
