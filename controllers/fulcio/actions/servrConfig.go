package actions

import (
	"context"
	"encoding/json"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/controllers/common/action"
	"github.com/securesign/operator/controllers/common/utils/kubernetes"
	"github.com/securesign/operator/controllers/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func NewServerConfigAction() action.Action[rhtasv1alpha1.Fulcio] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "create server config"
}

type FulcioMapConfig struct {
	OIDCIssuers map[string]rhtasv1alpha1.OIDCIssuer
	MetaIssuers map[string]rhtasv1alpha1.OIDCIssuer
}

func (i serverConfig) CanHandle(ctx context.Context, instance *rhtasv1alpha1.Fulcio) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	if c.Reason != constants.Creating && c.Reason != constants.Ready {
		return false
	}

	if instance.Status.ServerConfigRef == nil {
		return true
	}
	existing, err := kubernetes.GetConfigMap(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
	if err != nil {
		i.Logger.Error(err, "Cant load existing configuration")
		return false
	}
	expected, err := json.Marshal(ConvertToFulcioMapConfig(instance.Spec.Config))
	if err != nil {
		i.Logger.Error(err, "Cant parse expected configuration")
		return false
	}
	return existing.Data["config.json"] != string(expected)
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

	config, err := json.Marshal(ConvertToFulcioMapConfig(instance.Spec.Config))
	if err != nil {
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}
	expected := kubernetes.CreateImmutableConfigmap(fmt.Sprintf("fulcio-config-%s", instance.Name), instance.Namespace, labels, map[string]string{
		"config.json": string(config),
	})
	if err = controllerutil.SetControllerReference(instance, expected, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for ConfigMap: %w", err))
	}

	// invalidate config
	if instance.Status.ServerConfigRef != nil {
		if err = i.Client.Delete(ctx, &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instance.Status.ServerConfigRef.Name,
				Namespace: instance.Namespace,
			},
		}); err != nil {
			return i.Failed(err)
		}
		instance.Status.ServerConfigRef = nil
	}

	if err = i.Client.Create(ctx, expected); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CertCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return i.FailedWithStatusUpdate(ctx, err, instance)
	}

	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: expected.Name}

	i.Recorder.Event(instance, v1.EventTypeNormal, "FulcioConfigUpdated", "Fulcio config updated")
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{Type: constants.Ready,
		Status: metav1.ConditionFalse, Reason: constants.Creating, Message: "Server config created"})
	return i.StatusUpdate(ctx, instance)

}
