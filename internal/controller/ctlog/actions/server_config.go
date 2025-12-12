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
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	serverConfigResourceName = "ctlog-server-config"
)

// Annotations used to track the data sources for server config secret
var serverConfigAnnotations = []string{
	labels.LabelNamespace + "/treeID",
	labels.LabelNamespace + "/trillianUrl",
	labels.LabelNamespace + "/rootCertificates",
	labels.LabelNamespace + "/privateKeyRef",
}

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
	// Always run Handle() to validate the config secret exists and is valid
	return c != nil
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1alpha1.CTlog) *action.Result {
	var (
		err error
	)

	if instance.Spec.ServerConfigRef != nil {
		// Validate that the custom secret is accessible
		secret, err := kubernetes.GetSecret(i.Client, instance.Namespace, instance.Spec.ServerConfigRef.Name)
		if err != nil {
			return i.Error(ctx, fmt.Errorf("error accessing custom server config secret: %w", err), instance,
				metav1.Condition{
					Type:               ConfigCondition,
					Status:             metav1.ConditionFalse,
					Reason:             constants.Failure,
					Message:            fmt.Sprintf("Error accessing custom server config secret: %s", instance.Spec.ServerConfigRef.Name),
					ObservedGeneration: instance.Generation,
				})
		}
		if secret.Data == nil || secret.Data[ctlogUtils.ConfigKey] == nil {
			return i.Error(ctx, fmt.Errorf("custom server config secret is invalid"), instance,
				metav1.Condition{
					Type:               ConfigCondition,
					Status:             metav1.ConditionFalse,
					Reason:             constants.Failure,
					Message:            fmt.Sprintf("Custom server config secret is missing '%s' key: %s", ctlogUtils.ConfigKey, instance.Spec.ServerConfigRef.Name),
					ObservedGeneration: instance.Generation,
				})
		}

		instance.Status.ServerConfigRef = instance.Spec.ServerConfigRef
		i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigUpdated", "CTLog config updated: %s", instance.Spec.ServerConfigRef.Name)
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionTrue,
			Reason:             constants.Ready,
			Message:            "Using custom server config",
			ObservedGeneration: instance.Generation,
		})
		return i.StatusUpdate(ctx, instance)
	}

	// Validate prerequisites and normalize Trillian address before validation
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

	c := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
	isSpecChange := c != nil && c.ObservedGeneration != instance.Generation

	// Validate existing secret before attempting recreation (only for hot updates, not spec changes)
	if !isSpecChange && instance.Status.ServerConfigRef != nil && instance.Status.ServerConfigRef.Name != "" {
		valid, err := i.validateExistingSecret(instance, trillianUrl)
		if err != nil {
			// API error other than NotFound - fail reconciliation
			return i.Error(ctx, fmt.Errorf("error validating server config secret: %w", err), instance,
				metav1.Condition{
					Type:               ConfigCondition,
					Status:             metav1.ConditionFalse,
					Reason:             constants.Failure,
					Message:            fmt.Sprintf("Error accessing config secret: %s", instance.Status.ServerConfigRef.Name),
					ObservedGeneration: instance.Generation,
				})
		}
		if valid {
			return i.Continue()
		}
		// Secret needs recreation - log and continue
		i.Logger.Info("Server config secret needs recreation", "secret", instance.Status.ServerConfigRef.Name)
		i.Recorder.Eventf(instance, corev1.EventTypeWarning, "CTLogConfigRecreate", "Config secret will be recreated: %s", instance.Status.ServerConfigRef.Name)
	}

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
			Message:            "Waiting for Ctlog private key secret",
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

	configAnnotations := i.configMatchingAnnotations(instance, trillianUrl)

	if _, err = kubernetes.CreateOrUpdate(ctx, i.Client,
		newConfig,
		ensure.ControllerReference[*corev1.Secret](instance, i.Client),
		ensure.Labels[*corev1.Secret](slices.Collect(maps.Keys(configLabels)), configLabels),
		ensure.Annotations[*corev1.Secret](serverConfigAnnotations, configAnnotations),
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

	i.Logger.Info("Server config secret created", "secret", newConfig.Name)
	i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigCreated", "Config secret created successfully: %s", newConfig.Name)
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
			i.Recorder.Eventf(instance, corev1.EventTypeWarning, "CTLogConfigCleanupFailed", "Unable to delete old config secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid Secret with ctlog configuration", "Name", partialConfig.Name)
		i.Recorder.Eventf(instance, corev1.EventTypeNormal, "CTLogConfigCleanedUp", "Old config secret deleted successfully: %s", partialConfig.Name)
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
		certs = append(certs, data)
	}

	return certs, nil
}

// validateExistingSecret checks if the existing server config secret is valid.
// Returns:
//   - (true, nil) if the secret is valid and no recreation is needed
//   - (false, nil) if the secret needs recreation (NotFound or validation failed)
//   - (false, error) if there was an API error (other than NotFound) - reconciliation should fail
func (i serverConfig) validateExistingSecret(instance *rhtasv1alpha1.CTlog, trillianUrl string) (bool, error) {
	secret, err := kubernetes.GetSecret(i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Secret doesn't exist - needs recreation
			return false, nil
		}
		// Other API error (Forbidden, etc.) - fail reconciliation
		return false, err
	}

	// Check if the secret was generated from the same data sources using annotations
	expectedAnnotations := i.configMatchingAnnotations(instance, trillianUrl)
	if !equality.Semantic.DeepDerivative(expectedAnnotations, secret.GetAnnotations()) {
		return false, nil
	}

	// Secret is valid
	return true, nil
}

// configMatchingAnnotations generates annotations that identify the data sources
// used to generate the server config secret.
func (i serverConfig) configMatchingAnnotations(instance *rhtasv1alpha1.CTlog, trillianUrl string) map[string]string {
	// Build a string representation of root certificate references
	var rootCertRefs []string
	for _, ref := range instance.Status.RootCertificates {
		rootCertRefs = append(rootCertRefs, fmt.Sprintf("%s/%s", ref.Name, ref.Key))
	}

	annotations := map[string]string{
		labels.LabelNamespace + "/trillianUrl": trillianUrl,
	}

	if instance.Status.TreeID != nil {
		annotations[labels.LabelNamespace+"/treeID"] = fmt.Sprintf("%d", *instance.Status.TreeID)
	}

	if len(rootCertRefs) > 0 {
		annotations[labels.LabelNamespace+"/rootCertificates"] = strings.Join(rootCertRefs, ",")
	}

	if instance.Status.PrivateKeyRef != nil {
		annotations[labels.LabelNamespace+"/privateKeyRef"] = fmt.Sprintf("%s/%s", instance.Status.PrivateKeyRef.Name, instance.Status.PrivateKeyRef.Key)
	}

	return annotations
}
