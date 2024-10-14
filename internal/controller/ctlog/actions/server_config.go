package actions

import (
	"context"
	"fmt"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	utils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	serverConfigResourceName = "ctlog-server-config"
)

func NewServerConfigAction() action.Action[*rhtasv1alpha1.CTlog] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "server config"
}

func (i serverConfig) CanHandle(_ context.Context, instance *rhtasv1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)

	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	case instance.Status.ServerConfigRef == nil:
		return true
	case instance.Spec.ServerConfigRef != nil:
		return !equality.Semantic.DeepEqual(instance.Spec.ServerConfigRef, instance.Status.ServerConfigRef)
	case c.Reason == constants.Ready:
		return instance.Generation != c.ObservedGeneration
	default:
		return false
	}
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err error
	)

	if instance.Spec.ServerConfigRef != nil {
		instance.Status.ServerConfigRef = instance.Spec.ServerConfigRef
		i.Recorder.Event(instance, corev1.EventTypeNormal, "CTLogConfigUpdated", "CTLog config updated")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "CTLog config updated",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	}

	switch {
	case instance.Status.TreeID == nil:
		return i.Failed(fmt.Errorf("%s: %v", i.Name(), ctlogUtils.TreeNotSpecified))
	case instance.Status.PrivateKeyRef == nil:
		return i.Failed(fmt.Errorf("%s: %v", i.Name(), ctlogUtils.PrivateKeyNotSpecified))
	case instance.Spec.Trillian.Port == nil:
		return i.Failed(fmt.Errorf("%s: %v", i.Name(), ctlogUtils.TrillianPortNotSpecified))
	case instance.Spec.Trillian.Address == "":
		instance.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", trillian.LogserverDeploymentName, instance.Namespace)
	}

	trillianUrl := fmt.Sprintf("%s:%d", instance.Spec.Trillian.Address, *instance.Spec.Trillian.Port)

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
	labels[constants.LabelResource] = serverConfigResourceName

	// try to discover existing config and clear them out
	partialConfigs, err := utils.ListSecrets(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(labels).String())
	if err != nil {
		i.Logger.Error(err, "problem with listing configmaps", "namespace", instance.Namespace)
	}
	for _, partialConfig := range partialConfigs.Items {
		i.Logger.Info("Remove invalid Secret with ctlog configuration", "Name", partialConfig.Name)
		i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigDeleted", "Secret with ctlog configuration deleted: %s", partialConfig.Name)
		_ = i.Client.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: partialConfig.Name, Namespace: partialConfig.Namespace}})

	}

	rootCerts, err := i.handleRootCertificates(instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            fmt.Sprintf("Waiting for Fulcio root certificate: %v", err.Error()),
			ObservedGeneration: instance.Generation,
		})
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}

	certConfig, err := i.handlePrivateKey(instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			Message:            "Waiting for Ctlog private key secret",
			ObservedGeneration: instance.Generation,
		})
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}

	var cfg map[string][]byte
	if cfg, err = ctlogUtils.CreateCtlogConfig(trillianUrl, *instance.Status.TreeID, rootCerts, certConfig); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return i.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create CTLog configuration: %w", err), instance)
	}

	newConfig := utils.CreateImmutableSecret(fmt.Sprintf("ctlog-config-%s", instance.Name), instance.Namespace, cfg, labels)
	if err = controllerutil.SetControllerReference(instance, newConfig, i.Client.Scheme()); err != nil {
		return i.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}

	_, err = i.Ensure(ctx, newConfig)
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

	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	i.Recorder.Event(instance, corev1.EventTypeNormal, "CTLogConfigUpdated", "CTLog config updated")
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               constants.Ready,
		Status:             metav1.ConditionFalse,
		Reason:             constants.Creating,
		Message:            "Server config created",
		ObservedGeneration: instance.Generation,
	})
	return i.StatusUpdate(ctx, instance)
}

func (i serverConfig) handlePrivateKey(instance *rhtasv1alpha1.CTlog) (*ctlogUtils.KeyConfig, error) {
	if instance == nil {
		return nil, nil
	}
	private, err := utils.GetSecretData(i.Client, instance.Namespace, instance.Status.PrivateKeyRef)
	if err != nil {
		return nil, err
	}
	public, err := utils.GetSecretData(i.Client, instance.Namespace, instance.Status.PublicKeyRef)
	if err != nil {
		return nil, err
	}
	password, err := utils.GetSecretData(i.Client, instance.Namespace, instance.Status.PrivateKeyPasswordRef)
	if err != nil {
		return nil, err
	}

	return &ctlogUtils.KeyConfig{
		PrivateKey:     private,
		PublicKey:      public,
		PrivateKeyPass: password,
	}, nil
}

func (i serverConfig) handleRootCertificates(instance *rhtasv1alpha1.CTlog) ([]ctlogUtils.RootCertificate, error) {
	certs := make([]ctlogUtils.RootCertificate, 0)

	for _, selector := range instance.Status.RootCertificates {
		data, err := utils.GetSecretData(i.Client, instance.Namespace, &selector)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", selector.Name, selector.Key, err)
		}
		certs = append(certs, data)
	}

	return certs, nil
}
