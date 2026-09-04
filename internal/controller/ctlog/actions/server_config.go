package actions

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/serviceresolver"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels2 "k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
)

const (
	serverConfigResourceName = "ctlog-server-config"
)

// errSecretInvalid indicates the secret needs to be recreated (not a failure)
var errSecretInvalid = errors.New("secret needs recreation")

var serverConfigAnnotations = []string{
	labels.LabelNamespace + "/trillianUrl",
	labels.LabelNamespace + "/logsHash",
}

func NewServerConfigAction() action.Action[*rhtasv1.CTlog] {
	return &serverConfig{}
}

type serverConfig struct {
	action.BaseAction
}

func (i serverConfig) Name() string {
	return "server config"
}

func (i serverConfig) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
	// Always run Handle() to validate the config secret exists and is valid
	return c != nil
}

func (i serverConfig) Handle(ctx context.Context, instance *rhtasv1.CTlog) *action.Result {
	if len(instance.Status.Logs) == 0 {
		return i.Error(ctx, fmt.Errorf("%s: no logs in status", i.Name()), instance)
	}

	trillianHost, trillianPort, err := serviceresolver.ResolveInternalGrpcService(ctx, i.Client, instance.Spec.Trillian, instance.Namespace, &rhtasv1.Trillian{})
	if err != nil {
		return i.Error(ctx, fmt.Errorf("error resolving Trillian URL: %w", err), instance, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			Message:            fmt.Sprintf("Waiting for Trillian service to become available: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}
	trillianUrl := fmt.Sprintf("%s:%s", trillianHost, trillianPort)

	// Validate existing secret before attempting recreation
	if instance.Status.ServerConfigRef != nil && instance.Status.ServerConfigRef.Name != "" {
		if err := i.validateExistingSecret(ctx, instance, trillianUrl); err != nil {
			if errors.Is(err, errSecretInvalid) {
				i.Logger.Info("Server config secret needs recreation", "secret", instance.Status.ServerConfigRef.Name)
				i.Recorder.Eventf(instance, nil, corev1.EventTypeWarning, "CTLogConfigRecreate", "Recreating", "Config secret will be recreated: %s", instance.Status.ServerConfigRef.Name)
			} else {
				return i.Error(ctx, fmt.Errorf("error validating server config secret: %w", err), instance,
					metav1.Condition{
						Type:               ConfigCondition,
						Status:             metav1.ConditionFalse,
						Reason:             state.Failure.String(),
						Message:            fmt.Sprintf("Error accessing config secret: %s", instance.Status.ServerConfigRef.Name),
						ObservedGeneration: instance.Generation,
					})
			}
		} else {
			c := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
			isSpecChange := c != nil && c.ObservedGeneration != instance.Generation
			if isSpecChange {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               ConfigCondition,
					Status:             metav1.ConditionTrue,
					Reason:             c.Reason,
					Message:            c.Message,
					ObservedGeneration: instance.Generation,
				})
				return i.ReturnOnChange(i.PersistStatus)(ctx, instance)
			}
			return i.Continue()
		}
	}

	configLabels := labels.ForResource(ComponentName, DeploymentName, instance.Name, serverConfigResourceName)

	logs, err := i.resolveAllLogs(ctx, instance)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not resolve log entries: %w", err), instance, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            fmt.Sprintf("Failed to resolve log entries: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	cfg, hash, err := ctlogUtils.CreateConfig(trillianUrl, logs)
	if err != nil {
		return i.Error(ctx, fmt.Errorf("could not create CTLog configuration: %w", err), instance, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
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

	configAnnotations := i.configMatchingAnnotations(trillianUrl, hash)

	if err = kubernetes.Create(ctx, i.Client,
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
				Reason:             state.Failure.String(),
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
	}

	instance.Status.ServerConfigRef = &rhtasv1.LocalObjectReference{Name: newConfig.Name}

	i.Logger.Info("Server config secret created", "secret", newConfig.Name)
	i.Recorder.Eventf(instance, newConfig, corev1.EventTypeNormal, "CTLogConfigCreated", "Created", "Config secret created successfully: %s", newConfig.Name)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               ConfigCondition,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		Message:            "Server config created", //nolint:goconst
		ObservedGeneration: instance.Generation,
	})
	changed, err := i.PersistStatus(ctx, instance)
	if err != nil {
		return i.Error(ctx, err, instance)
	}
	i.cleanup(ctx, instance, configLabels)
	if changed {
		return i.Return()
	}
	return i.Continue()
}

// resolveAllLogs builds a ShardConfig for every entry in status.logs,
// reading secret data as needed. Active/inactive is irrelevant here —
// every entry is serialized uniformly into the config proto.
func (i serverConfig) resolveAllLogs(ctx context.Context, instance *rhtasv1.CTlog) ([]ctlogUtils.ShardConfig, error) {
	logs := make([]ctlogUtils.ShardConfig, 0, len(instance.Status.Logs))

	for _, log := range instance.Status.Logs {
		specLog := ctlogUtils.GetLog(log.Prefix, instance.Spec.Logs)
		if log.LogId == nil {
			return nil, fmt.Errorf("log %q has no LogId", log.Prefix)
		}

		sc := ctlogUtils.ShardConfig{
			TreeID:   *log.LogId,
			Prefix:   log.Prefix,
			Readonly: ptr.Deref(specLog.Readonly, false),
		}

		// Root certificates
		for _, selector := range log.RootCertificates {
			data, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, &selector)
			if err != nil {
				return nil, fmt.Errorf("log %q root cert %s/%s: %w", log.Prefix, selector.Name, selector.Key, err)
			}
			sc.RootCerts = append(sc.RootCerts, data)
		}

		// Public key
		if log.PublicKeyRef != nil {
			publicKey, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, log.PublicKeyRef)
			if err != nil {
				return nil, fmt.Errorf("log %q publicKeyRef: %w", log.Prefix, err)
			}
			sc.PublicKey = publicKey
		}

		switch specLog.Signer.Type {
		case rhtasv1.SignerTypePKCS11:
			pkcsConfig := specLog.Signer.PKCS11
			if pkcsConfig == nil {
				return nil, fmt.Errorf("log %q: configuration for PKCS#11 has not been set", log.Prefix)
			}
			if pkcsConfig.PinSecretRef == nil {
				return nil, fmt.Errorf("log %q: pinSecretRef is required for PKCS#11 signer", log.Prefix)
			}
			if log.PublicKeyRef == nil {
				return nil, fmt.Errorf("log %q: publicKeyRef is required for PKCS#11 signer", log.Prefix)
			}
			pin, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, pkcsConfig.PinSecretRef)
			if err != nil {
				return nil, fmt.Errorf("log %q pkcs11 pinSecretRef: %w", log.Prefix, err)
			}
			if len(pin) == 0 {
				return nil, fmt.Errorf("log %q: PIN secret %s/%s is empty", log.Prefix, pkcsConfig.PinSecretRef.Name, pkcsConfig.PinSecretRef.Key)
			}
			sc.PKCS11 = &ctlogUtils.PKCS11ShardConfig{
				TokenLabel: pkcsConfig.TokenLabel,
				Pin:        string(pin),
			}
		case rhtasv1.SignerTypeFile:
			if log.PrivateKeyRef != nil {
				privateKey, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, log.PrivateKeyRef)
				if err != nil {
					return nil, fmt.Errorf("log %q privateKeyRef: %w", log.Prefix, err)
				}
				sc.PrivateKey = privateKey
			}
		default:
			return nil, fmt.Errorf("log %q: signer type %q is not supported", log.Prefix, specLog.Signer.Type)
		}

		if specLog.FrozenSTH != nil {
			sc.FrozenSTH = &ctlogUtils.FrozenSTH{
				Sha256RootHash:    specLog.FrozenSTH.Sha256RootHash,
				TreeHeadSignature: specLog.FrozenSTH.TreeHeadSignature,
			}
			if specLog.FrozenSTH.TreeSize != nil {
				sc.FrozenSTH.TreeSize = *specLog.FrozenSTH.TreeSize
			}
			if specLog.FrozenSTH.Timestamp != nil {
				sc.FrozenSTH.Timestamp = specLog.FrozenSTH.Timestamp.Unix()
			}
		}

		if specLog.NotAfterStart != nil {
			sc.NotAfterStart = specLog.NotAfterStart.Unix()
		}
		if specLog.NotAfterLimit != nil {
			sc.NotAfterLimit = specLog.NotAfterLimit.Unix()
		}

		logs = append(logs, sc)
	}

	return logs, nil
}

func (i serverConfig) cleanup(ctx context.Context, instance *rhtasv1.CTlog, configLabels map[string]string) {
	if instance.Status.ServerConfigRef == nil || instance.Status.ServerConfigRef.Name == "" {
		i.Logger.Error(errors.New("new Secret name is empty"), "unable to clean old objects", "namespace", instance.Namespace)
		return
	}

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
			i.Recorder.Eventf(instance, nil, corev1.EventTypeWarning, "CTLogConfigCleanupFailed", "CleanupFailed", "Unable to delete old config secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid Secret with ctlog configuration", "Name", partialConfig.Name)
		i.Recorder.Eventf(instance, nil, corev1.EventTypeNormal, "CTLogConfigCleanedUp", "Deleted", "Old config secret deleted successfully: %s", partialConfig.Name)
	}
}

func (i serverConfig) validateExistingSecret(ctx context.Context, instance *rhtasv1.CTlog, trillianUrl string) error {
	// Cert rotation updates the root cert secret content but keeps the same
	// secret reference, so the logsHash annotation won't detect the change.
	// handleFulcioCert signals this by setting ConfigCondition to False with
	// FulcioReason — honor that signal and force recreation.
	c := meta.FindStatusCondition(instance.Status.Conditions, ConfigCondition)
	if c != nil && c.Status == metav1.ConditionFalse && c.Reason == FulcioReason {
		return errSecretInvalid
	}

	secretMeta, err := kubernetes.GetSecretMetadata(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return errSecretInvalid
		}
		return err
	}

	logs, err := i.resolveAllLogs(ctx, instance)
	if err != nil {
		return err
	}
	_, hash, err := ctlogUtils.CreateConfig(trillianUrl, logs)
	if err != nil {
		return err
	}
	expectedAnnotations := i.configMatchingAnnotations(trillianUrl, hash)
	actualAnnotations := secretMeta.GetAnnotations()
	for _, key := range serverConfigAnnotations {
		expected, hasExpected := expectedAnnotations[key]
		actual, hasActual := actualAnnotations[key]
		if hasExpected != hasActual || expected != actual {
			return errSecretInvalid
		}
	}

	return nil
}

// configMatchingAnnotations generates annotations that identify the data
// sources used to generate the server config secret. All status.logs entries
// are hashed uniformly — no active/inactive distinction.
func (i serverConfig) configMatchingAnnotations(trillianUrl string, configHash [32]byte) map[string]string {
	ann := map[string]string{
		labels.LabelNamespace + "/trillianUrl": trillianUrl,
	}
	ann[labels.LabelNamespace+"/logsHash"] = hex.EncodeToString(configHash[:])

	return ann
}
