package actions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	trillian "github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/labels"
	cryptoutil "github.com/securesign/operator/internal/utils/crypto"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	c := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)

	switch {
	case c == nil:
		return false
	case !meta.IsStatusConditionTrue(instance.Status.Conditions, ConfigCondition):
		return true
	case instance.Status.ServerConfigRef == nil:
		return true
	case instance.Spec.ServerConfigRef != nil:
		return !equality.Semantic.DeepEqual(instance.Spec.ServerConfigRef, instance.Status.ServerConfigRef)
	default:
		return instance.Generation != c.ObservedGeneration
	}
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err error
	)

	if instance.Spec.ServerConfigRef != nil {
		if cryptoutil.FIPSEnabled {
			if err := validateExternalConfig(i.Client, instance); err != nil {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               ConfigCondition,
					Status:             metav1.ConditionFalse,
					Reason:             constants.Failure,
					Message:            fmt.Sprintf("Invalid server config: %v", err),
					ObservedGeneration: instance.Generation,
				})
				i.StatusUpdate(ctx, instance)
				return i.Requeue()
			}
		}
		instance.Status.ServerConfigRef = instance.Spec.ServerConfigRef
		i.Recorder.Event(instance, corev1.EventTypeNormal, "CTLogConfigUpdated", "CTLog config updated")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionTrue,
			Reason:             constants.Ready,
			Message:            "Using custom server config",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	}

	switch {
	case instance.Status.TreeID == nil:
		return i.Error(ctx, fmt.Errorf("%s: %v", i.Name(), ctlogUtils.ErrTreeNotSpecified), instance)
	case instance.Status.PrivateKeyRef == nil:
		return i.Error(ctx, fmt.Errorf("%s: %v", i.Name(), ctlogUtils.ErrPrivateKeyNotSpecified), instance)
	case instance.Spec.Trillian.Port == nil:
		return i.Error(ctx, reconcile.TerminalError(fmt.Errorf("%s: %v", i.Name(), ctlogUtils.ErrTrillianPortNotSpecified)), instance)
	case instance.Spec.Trillian.Address == "":
		instance.Spec.Trillian.Address = fmt.Sprintf("%s.%s.svc", trillian.LogserverDeploymentName, instance.Namespace)
	}

	trillianUrl := fmt.Sprintf("%s:%d", instance.Spec.Trillian.Address, *instance.Spec.Trillian.Port)

	configLabels := labels.ForResource(ComponentName, DeploymentName, instance.Name, serverConfigResourceName)

	rootCerts, err := i.handleRootCertificates(instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             FulcioReason,
			Message:            fmt.Sprintf("Waiting for Fulcio root certificate: %v", err.Error()),
			ObservedGeneration: instance.Generation,
		})
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}

	certConfig, err := i.handlePrivateKey(instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             SignerKeyReason,
			Message:            fmt.Sprintf("Waiting for Ctlog private key secret: %v", err),
			ObservedGeneration: instance.Generation,
		})
		i.StatusUpdate(ctx, instance)
		return i.Requeue()
	}

	var cfg map[string][]byte
	if cfg, err = ctlogUtils.CreateCtlogConfig(trillianUrl, *instance.Status.TreeID, rootCerts, certConfig); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create CTLog configuration: %w", err), instance, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
	}

	newConfig := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("ctlog-config-%s", instance.Name),
			Namespace:    instance.Namespace,
		},
	}

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		newConfig,
		ensure.ControllerReference[*corev1.Secret](instance, i.Client),
		ensure.Labels[*corev1.Secret](slices.Collect(maps.Keys(configLabels)), configLabels),
		kubernetes.EnsureSecretData(true, cfg),
	); err != nil {
		return i.Error(ctx, fmt.Errorf("could not create Server config: %w", err), instance,
			metav1.Condition{
				Type:               ConfigCondition,
				Status:             metav1.ConditionFalse,
				Reason:             constants.Failure,
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
	}

	instance.Status.ServerConfigRef = &rhtasv1alpha1.LocalObjectReference{Name: newConfig.Name}

	i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigCreated", "Secret with ctlog configuration created: %s", newConfig.Name)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               ConfigCondition,
		Status:             metav1.ConditionTrue,
		Reason:             constants.Ready,
		Message:            "Server config created",
		ObservedGeneration: instance.Generation,
	})
	result := i.StatusUpdate(ctx, instance)
	if action.IsSuccess(result) {
		i.cleanup(ctx, instance, configLabels)
	}
	return result
}

func (i serverConfig) cleanup(ctx context.Context, instance *rhtasv1alpha1.CTlog, configLabels map[string]string) {
	if instance.Status.ServerConfigRef == nil || instance.Status.ServerConfigRef.Name == "" {
		i.Logger.Error(errors.New("new Secret name is empty"), "unable to clean old objects", "namespace", instance.Namespace)
		return
	}

	// try to discover existing secrets and clear them out
	partialConfigs, err := kubernetes.ListSecrets(ctx, i.Client, instance.Namespace, labels2.SelectorFromSet(configLabels).String())
	if err != nil {
		i.Logger.Error(err, "problem with listing configmaps", "namespace", instance.Namespace)
		return
	}
	for _, partialConfig := range partialConfigs.Items {
		if partialConfig.Name == instance.Status.ServerConfigRef.Name {
			continue
		}

		err = i.Client.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: partialConfig.Name, Namespace: partialConfig.Namespace}})
		if err != nil {
			i.Logger.Error(err, "unable to delete secret", "namespace", instance.Namespace, "name", partialConfig.Name)
			i.Recorder.Eventf(instance, corev1.EventTypeWarning, "CTLogConfigDeleted", "Unable to delete secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid Secret with ctlog configuration", "Name", partialConfig.Name)
		i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigDeleted", "Secret with ctlog configuration deleted: %s", partialConfig.Name)
	}
}

func (i serverConfig) handlePrivateKey(instance *rhtasv1alpha1.CTlog) (*ctlogUtils.KeyConfig, error) {
	if instance == nil {
		return nil, nil
	}
	private, err := kubernetes.GetSecretData(i.Client, instance.Namespace, instance.Status.PrivateKeyRef)
	if err != nil {
		return nil, err
	}
	public, err := kubernetes.GetSecretData(i.Client, instance.Namespace, instance.Status.PublicKeyRef)
	if err != nil {
		return nil, err
	}
	password, err := kubernetes.GetSecretData(i.Client, instance.Namespace, instance.Status.PrivateKeyPasswordRef)
	if err != nil {
		return nil, err
	}

	if cryptoutil.FIPSEnabled {
		if err := cryptoutil.ValidatePrivateKeyPEM(private, password); err != nil {
			return nil, err
		}
		if err := cryptoutil.ValidatePublicKeyPEM(public); err != nil {
			return nil, err
		}
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
		data, err := kubernetes.GetSecretData(i.Client, instance.Namespace, &selector)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", selector.Name, selector.Key, err)
		}
		if cryptoutil.FIPSEnabled {
			if err := cryptoutil.ValidateCertificatePEM(data); err != nil {
				return nil, fmt.Errorf("%s/%s: %w", selector.Name, selector.Key, err)
			}
		}
		certs = append(certs, data)
	}

	return certs, nil
}

func validateExternalConfig(cli client.Client, instance *rhtasv1alpha1.CTlog) error {
	secret, err := kubernetes.GetSecret(cli, instance.Namespace, instance.Spec.ServerConfigRef.Name)
	if err != nil {
		return fmt.Errorf("could not retrieve server config secret %s: %w", instance.Spec.ServerConfigRef.Name, err)
	}

	if key, ok := secret.Data[ctlogUtils.PrivateKey]; ok {
		if err := cryptoutil.ValidatePrivateKeyPEM(key, secret.Data[ctlogUtils.Password]); err != nil {
			return fmt.Errorf("private key is not FIPS-compliant: %w", err)
		}
	}

	if key, ok := secret.Data[ctlogUtils.PublicKey]; ok {
		if err := cryptoutil.ValidatePublicKeyPEM(key); err != nil {
			return fmt.Errorf("public key is not FIPS-compliant: %w", err)
		}
	}

	for k, v := range secret.Data {
		if strings.HasPrefix(k, "fulcio-") {
			if err := cryptoutil.ValidateCertificatePEM(v); err != nil {
				return fmt.Errorf("root certificate %s is not FIPS-compliant: %w", k, err)
			}
		}
	}

	return nil
}
