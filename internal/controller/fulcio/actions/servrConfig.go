package actions

import (
	"context"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v2"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	configResourceLabel = "server-config"
	serverConfigName    = "config.yaml"
)

func NewServerConfigAction() action.Action[*rhtasv1alpha1.Fulcio] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "create server config"
}

type FulcioMapConfig struct {
	OIDCIssuers map[string]rhtasv1alpha1.OIDCIssuer `yaml:"oidc-issuers"`
	MetaIssuers map[string]rhtasv1alpha1.OIDCIssuer `yaml:"meta-issuers"`
}

func (i serverConfig) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	default:
		return true
	}

}

func ConvertToFulcioMapConfig(fulcioConfig rhtasv1alpha1.FulcioConfig) *FulcioMapConfig {
	OIDCIssuers := make(map[string]rhtasv1alpha1.OIDCIssuer)
	MetaIssuers := make(map[string]rhtasv1alpha1.OIDCIssuer)

	for _, issuer := range fulcioConfig.OIDCIssuers {
		OIDCIssuers[issuer.Issuer] = issuer
	}

	for _, issuer := range fulcioConfig.MetaIssuers {
		MetaIssuers[issuer.Issuer] = issuer
	}

	fulcioMapConfig := &FulcioMapConfig{
		OIDCIssuers: OIDCIssuers,
		MetaIssuers: MetaIssuers,
	}
	return fulcioMapConfig
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) *action.Result {
	var (
		err error
	)
	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
	labels[constants.LabelResource] = configResourceLabel

	config, err := yaml.Marshal(ConvertToFulcioMapConfig(instance.Spec.Config))
	if err != nil {
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	// verify existing config
	if instance.Status.ServerConfigRef != nil {
		cfg, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
		if client.IgnoreNotFound(err) != nil {
			return i.Failed(fmt.Errorf("FulcioConfig: %w", err))
		}
		if cfg != nil {
			if reflect.DeepEqual(cfg.Data[serverConfigName], string(config)) {
				return i.Continue()
			} else {
				i.Logger.Info("Remove invalid ConfigMap with fulcio-server configuration", "Name", cfg.Name)
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
	for _, partialConfig := range partialConfigs.Items {
		cm, err := kubernetes.GetConfigMap(ctx, i.Client, partialConfig.Namespace, partialConfig.Name)
		if err != nil {
			return i.Failed(fmt.Errorf("can't load configMap data %w", err))
		}
		if reflect.DeepEqual(cm.Data[serverConfigName], string(config)) && instance.Status.ServerConfigRef == nil {
			i.Recorder.Eventf(instance, v1.EventTypeNormal, "FulcioConfigDiscovered", "Existing ConfigMap with fulcio configuration discovered: %s", cm.Name)
			instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: cm.Name}
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Creating,
				Message: "Server config discovered"})
		} else {
			i.Logger.Info("Remove invalid ConfigMap with rekor-server configuration", "Name", cm.Name)
			_ = i.Client.Delete(ctx, cm)
		}
	}
	if instance.Status.ServerConfigRef != nil {
		return i.StatusUpdate(ctx, instance)
	}

	// create new config
	newConfig := kubernetes.CreateImmutableConfigmap("fulcio-config-", instance.Namespace, labels, map[string]string{
		serverConfigName: string(config)})
	if err = controllerutil.SetControllerReference(instance, newConfig, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("FulcioConfig: could not set controller reference for ConfigMap: %w", err))
	}

	_, err = i.Ensure(ctx, newConfig)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}
	i.Recorder.Event(instance, v1.EventTypeNormal, "FulcioConfigUpdated", "Fulcio config updated")
	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	meta.SetStatusCondition(&instance.Status.Conditions,
		metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Server config created"},
	)
	return i.StatusUpdate(ctx, instance)
}
