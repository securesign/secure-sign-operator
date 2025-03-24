package actions

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/labels"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	configLabel := labels.ForResource(ComponentName, DeploymentName, instance.Name, configResourceLabel)

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

	// create new config
	newConfig := kubernetes.CreateImmutableConfigmap("fulcio-config-", instance.Namespace, configLabel, map[string]string{
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

	i.Recorder.Eventf(instance, v1.EventTypeNormal, "FulcioConfigUpdated", "Fulcio config updated: %s", newConfig.Name)
	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	meta.SetStatusCondition(&instance.Status.Conditions,
		metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Creating,
			Message: "Server config created"},
	)

	result := i.StatusUpdate(ctx, instance)

	if result.Result.IsZero() {
		i.cleanup(ctx, instance, configLabel)
	}
	return result
}

func (i serverConfig) cleanup(ctx context.Context, instance *rhtasv1alpha1.Fulcio, configLabels map[string]string) {
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
			i.Recorder.Eventf(instance, v1.EventTypeWarning, "FulcioConfigDeleted", "Unable to delete secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid ConfigMap with Fulcio configuration", "name", partialConfig.Name)
		i.Recorder.Eventf(instance, v1.EventTypeNormal, "FulcioConfigDeleted", "Fulcio config deleted: %s", partialConfig.Name)
	}
}
