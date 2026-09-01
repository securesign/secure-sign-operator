package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

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
)

const (
	serverConfigResourceName = "ctlog-server-config"
)

// errSecretInvalid indicates the secret needs to be recreated (not a failure)
var errSecretInvalid = errors.New("secret needs recreation")

// Annotations used to track the data sources for server config secret
var serverConfigAnnotations = []string{
	labels.LabelNamespace + "/treeID",
	labels.LabelNamespace + "/trillianUrl",
	labels.LabelNamespace + "/rootCertificatesHash",
	labels.LabelNamespace + "/privateKeyRef",
	labels.LabelNamespace + "/logPrefix",
	labels.LabelNamespace + "/pkcs11SpecHash",
	labels.LabelNamespace + "/shardingHash",
	labels.LabelNamespace + "/activeLogValidity",
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
	var (
		err error
	)

	// Validate prerequisites and normalize Trillian address before validation
	switch {
	case instance.Status.TreeID == nil:
		return i.Error(ctx, fmt.Errorf("%s: %v", i.Name(), ctlogUtils.ErrTreeNotSpecified), instance)
	case getSignerType(instance) == rhtasv1.SignerTypeFile && instance.Status.PrivateKeyRef == nil:
		return i.Error(ctx, fmt.Errorf("%s: %v", i.Name(), ctlogUtils.ErrPrivateKeyNotSpecified), instance)
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
				// Secret needs recreation - log and continue
				i.Logger.Info("Server config secret needs recreation", "secret", instance.Status.ServerConfigRef.Name)
				i.Recorder.Eventf(instance, nil, corev1.EventTypeWarning, "CTLogConfigRecreate", "Recreating", "Config secret will be recreated: %s", instance.Status.ServerConfigRef.Name)
			} else {
				// API error - fail reconciliation
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
			// Secret is valid - update observedGeneration if spec changed (e.g. replicas-only change)
			// to prevent unnecessary recreation on next reconciliation
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

	rootCerts, err := i.handleRootCertificates(ctx, instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             FulcioReason,
			Message:            fmt.Sprintf("Waiting for Fulcio root certificate: %v", err.Error()),
			ObservedGeneration: instance.Generation,
		})
		if _, err := i.PersistStatus(ctx, instance); err != nil {
			return i.Error(ctx, err, instance)
		}
		return i.RequeueAfter(5 * time.Second)
	}

	shards, err := i.handleShards(ctx, instance)
	if err != nil {
		return i.Error(ctx, err, instance, metav1.Condition{
			Type:               ConfigCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Failure.String(),
			Message:            fmt.Sprintf("Failed to resolve shard secrets: %v", err),
			ObservedGeneration: instance.Generation,
		})
	}

	isPKCS11 := getSignerType(instance) == rhtasv1.SignerTypePKCS11

	var cfg map[string][]byte
	if isPKCS11 {
		cfg, err = i.buildPKCS11Config(ctx, instance, trillianUrl, rootCerts)
	} else {
		certConfig, keyErr := i.handlePrivateKey(ctx, instance)
		if keyErr != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               ConfigCondition,
				Status:             metav1.ConditionFalse,
				Reason:             SignerKeyReason,
				Message:            "Waiting for Ctlog private key secret",
				ObservedGeneration: instance.Generation,
			})
			if _, err := i.PersistStatus(ctx, instance); err != nil {
				return i.Error(ctx, err, instance)
			}
			return i.RequeueAfter(5 * time.Second)
		}
		notAfterStart := int64(0)
		notAfterLimit := int64(0)
		activeLog := ctlogUtils.ActiveLog(instance.Spec.Logs)
		if activeLog != nil && activeLog.NotAfterStart != nil {
			notAfterStart = activeLog.NotAfterStart.Unix()
		}
		if activeLog != nil && activeLog.NotAfterLimit != nil {
			notAfterLimit = activeLog.NotAfterLimit.Unix()
		}
		logPrefix := ""
		if activeLog != nil {
			logPrefix = activeLog.Prefix
		}
		cfg, err = ctlogUtils.CreateCtlogConfig(trillianUrl, *instance.Status.TreeID, rootCerts, certConfig, logPrefix, shards, notAfterStart, notAfterLimit)
	}
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

	configAnnotations := i.configMatchingAnnotations(ctx, instance, trillianUrl)

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

func (i serverConfig) cleanup(ctx context.Context, instance *rhtasv1.CTlog, configLabels map[string]string) {
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
			i.Recorder.Eventf(instance, nil, corev1.EventTypeWarning, "CTLogConfigCleanupFailed", "CleanupFailed", "Unable to delete old config secret: %s", partialConfig.Name)
			continue
		}
		i.Logger.Info("Remove invalid Secret with ctlog configuration", "Name", partialConfig.Name)
		i.Recorder.Eventf(instance, nil, corev1.EventTypeNormal, "CTLogConfigCleanedUp", "Deleted", "Old config secret deleted successfully: %s", partialConfig.Name)
	}
}

func (i serverConfig) buildPKCS11Config(
	ctx context.Context,
	instance *rhtasv1.CTlog,
	trillianUrl string,
	rootCerts []ctlogUtils.RootCertificate,
) (map[string][]byte, error) {
	activeLog := ctlogUtils.ActiveLog(instance.Spec.Logs)
	var p *rhtasv1.CTlogPKCS11Config
	if activeLog != nil && activeLog.Signer != nil {
		p = activeLog.Signer.PKCS11
	}
	if p == nil {
		return nil, fmt.Errorf("PKCS#11 config is nil")
	}
	if p.PinSecretRef == nil {
		return nil, fmt.Errorf("pinSecretRef is required for PKCS#11 signer")
	}
	if p.PublicKeyRef == nil {
		return nil, fmt.Errorf("publicKeyRef is required for PKCS#11 signer")
	}

	pin, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, p.PinSecretRef)
	if err != nil {
		return nil, fmt.Errorf("failed to read PIN secret: %w", err)
	}
	if len(pin) == 0 {
		return nil, fmt.Errorf("PIN secret %s/%s is empty", p.PinSecretRef.Name, p.PinSecretRef.Key)
	}

	publicKey, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, p.PublicKeyRef)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key secret: %w", err)
	}
	if len(publicKey) == 0 {
		return nil, fmt.Errorf("public key secret %s/%s is empty", p.PublicKeyRef.Name, p.PublicKeyRef.Key)
	}

	logPrefix := ""
	if activeLog != nil {
		logPrefix = activeLog.Prefix
	}

	return ctlogUtils.CreateCtlogPKCS11Config(
		trillianUrl,
		*instance.Status.TreeID,
		rootCerts,
		p.TokenLabel,
		string(pin),
		publicKey,
		logPrefix,
	)
}

func (i serverConfig) handlePrivateKey(ctx context.Context, instance *rhtasv1.CTlog) (*ctlogUtils.KeyConfig, error) {
	if instance == nil {
		return nil, nil
	}
	private, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, instance.Status.PrivateKeyRef)
	if err != nil {
		return nil, err
	}
	public, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, instance.Status.PublicKeyRef)
	if err != nil {
		return nil, err
	}
	// Prefer spec-level PrivateKeyPasswordRef (from the active log's signer config),
	// then fall back to the deprecated status-level ref for backward compatibility.
	var passwordRef *rhtasv1.SecretKeySelector
	activeLog := ctlogUtils.ActiveLog(instance.Spec.Logs)
	if activeLog != nil && activeLog.Signer != nil && activeLog.Signer.File != nil && activeLog.Signer.File.PrivateKeyPasswordRef != nil {
		passwordRef = activeLog.Signer.File.PrivateKeyPasswordRef
	} else {
		passwordRef = instance.Status.PrivateKeyPasswordRef
	}
	password, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, passwordRef)
	if err != nil {
		return nil, err
	}

	return &ctlogUtils.KeyConfig{
		PrivateKey:     private,
		PublicKey:      public,
		PrivateKeyPass: password,
	}, nil
}

func (i serverConfig) handleRootCertificates(ctx context.Context, instance *rhtasv1.CTlog) ([]ctlogUtils.RootCertificate, error) {
	certs := make([]ctlogUtils.RootCertificate, 0)

	for _, selector := range instance.Status.RootCertificates {
		data, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, &selector)
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", selector.Name, selector.Key, err)
		}
		certs = append(certs, data)
	}

	return certs, nil
}

// validateExistingSecret checks if the existing server config secret is valid.
// Returns:
//   - nil if the secret is valid
//   - errSecretInvalid if the secret needs recreation (not a failure)
//   - other error for API errors - reconciliation should fail
func (i serverConfig) handleShards(ctx context.Context, instance *rhtasv1.CTlog) ([]ctlogUtils.ShardConfig, error) {
	// Collect all non-active logs
	nonActiveLogs := make([]rhtasv1.CTLogConfig, 0)
	for _, log := range instance.Spec.Logs {
		if log.Active == nil || !*log.Active {
			nonActiveLogs = append(nonActiveLogs, log)
		}
	}

	shards := make([]ctlogUtils.ShardConfig, 0, len(nonActiveLogs))
	for _, log := range nonActiveLogs {
		var publicKey, privateKey, privateKeyPassword []byte
		var pkcs11Cfg *ctlogUtils.PKCS11ShardConfig

		if log.Signer != nil && log.Signer.File != nil {
			if log.Signer.File.PublicKeyRef != nil {
				var err error
				publicKey, err = kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, log.Signer.File.PublicKeyRef)
				if err != nil {
					return nil, fmt.Errorf("shard %s publicKeyRef: %w", log.Prefix, err)
				}
			}
			if log.Signer.File.PrivateKeyRef != nil {
				var err error
				privateKey, err = kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, log.Signer.File.PrivateKeyRef)
				if err != nil {
					return nil, fmt.Errorf("shard %s privateKeyRef: %w", log.Prefix, err)
				}
			}
			if log.Signer.File.PrivateKeyPasswordRef != nil {
				var err error
				privateKeyPassword, err = kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, log.Signer.File.PrivateKeyPasswordRef)
				if err != nil {
					return nil, fmt.Errorf("shard %s privateKeyPasswordRef: %w", log.Prefix, err)
				}
			}
		}
		if log.Signer != nil && log.Signer.PKCS11 != nil {
			if log.Signer.PKCS11.PublicKeyRef != nil {
				var err error
				publicKey, err = kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, log.Signer.PKCS11.PublicKeyRef)
				if err != nil {
					return nil, fmt.Errorf("shard %s pkcs11.publicKeyRef: %w", log.Prefix, err)
				}
			}
			pin, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, log.Signer.PKCS11.PinSecretRef)
			if err != nil {
				return nil, fmt.Errorf("shard %s pkcs11.pinSecretRef: %w", log.Prefix, err)
			}
			pkcs11Cfg = &ctlogUtils.PKCS11ShardConfig{
				TokenLabel: log.Signer.PKCS11.TokenLabel,
				Pin:        string(pin),
			}
		}

		var frozenSTH *ctlogUtils.FrozenSTH
		if log.FrozenSTH != nil {
			frozenSTH = &ctlogUtils.FrozenSTH{
				Sha256RootHash:    log.FrozenSTH.Sha256RootHash,
				TreeHeadSignature: log.FrozenSTH.TreeHeadSignature,
			}
			if log.FrozenSTH.TreeSize != nil {
				frozenSTH.TreeSize = *log.FrozenSTH.TreeSize
			}
			if log.FrozenSTH.Timestamp != nil {
				frozenSTH.Timestamp = log.FrozenSTH.Timestamp.Unix()
			}
		}

		treeID := int64(0)
		if log.LogId != nil {
			treeID = *log.LogId
		}

		sc := ctlogUtils.ShardConfig{
			TreeID:             treeID,
			PublicKey:          publicKey,
			PrivateKey:         privateKey,
			PrivateKeyPassword: privateKeyPassword,
			PKCS11:             pkcs11Cfg,
			Prefix:             log.Prefix,
			FrozenSTH:          frozenSTH,
			Readonly:           log.Readonly != nil && *log.Readonly,
		}

		if len(log.RootCerts) > 0 {
			for _, selector := range log.RootCerts {
				data, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, &selector)
				if err != nil {
					return nil, fmt.Errorf("shard %s root cert %s/%s: %w", log.Prefix, selector.Name, selector.Key, err)
				}
				sc.RootCerts = append(sc.RootCerts, data)
			}
		}

		// Set validity timestamps if provided
		if log.NotAfterStart != nil {
			sc.NotAfterStart = log.NotAfterStart.Unix()
		}
		if log.NotAfterLimit != nil {
			sc.NotAfterLimit = log.NotAfterLimit.Unix()
		}

		shards = append(shards, sc)
	}
	return shards, nil
}

func (i serverConfig) validateExistingSecret(ctx context.Context, instance *rhtasv1.CTlog, trillianUrl string) error {
	secretMeta, err := kubernetes.GetSecretMetadata(ctx, i.Client, instance.Namespace, instance.Status.ServerConfigRef.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return errSecretInvalid
		}
		return err
	}

	// Check if the secret was generated from the same data sources using annotations.
	// Compare all tracked annotation keys exactly — both additions and removals
	// of data sources must trigger recreation.
	expectedAnnotations := i.configMatchingAnnotations(ctx, instance, trillianUrl)
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

// configMatchingAnnotations generates annotations that identify the data sources
// used to generate the server config secret.
func (i serverConfig) configMatchingAnnotations(ctx context.Context, instance *rhtasv1.CTlog, trillianUrl string) map[string]string {
	annotations := map[string]string{
		labels.LabelNamespace + "/trillianUrl": trillianUrl,
	}

	if instance.Status.TreeID != nil {
		annotations[labels.LabelNamespace+"/treeID"] = fmt.Sprintf("%d", *instance.Status.TreeID)
	}

	if certs, err := i.handleRootCertificates(ctx, instance); err == nil {
		h := sha256.New()
		for _, cert := range certs {
			h.Write(cert)
		}
		annotations[labels.LabelNamespace+"/rootCertificatesHash"] = hex.EncodeToString(h.Sum(nil))
	} else {
		annotations[labels.LabelNamespace+"/rootCertificatesHash"] = "unresolvable"
	}

	if instance.Status.PrivateKeyRef != nil {
		annotations[labels.LabelNamespace+"/privateKeyRef"] = fmt.Sprintf("%s/%s", instance.Status.PrivateKeyRef.Name, instance.Status.PrivateKeyRef.Key)
	}

	activeLog := ctlogUtils.ActiveLog(instance.Spec.Logs)
	if activeLog != nil && activeLog.Signer != nil && activeLog.Signer.File != nil && activeLog.Signer.File.PrivateKeyPasswordRef != nil {
		ref := activeLog.Signer.File.PrivateKeyPasswordRef
		annotations[labels.LabelNamespace+"/privateKeyPasswordRef"] = fmt.Sprintf("%s/%s", ref.Name, ref.Key)
	}
	if activeLog != nil && activeLog.Prefix != "" {
		annotations[labels.LabelNamespace+"/logPrefix"] = activeLog.Prefix
	}

	if activeLog != nil && (activeLog.NotAfterStart != nil || activeLog.NotAfterLimit != nil) {
		h := sha256.New()
		if activeLog.NotAfterStart != nil {
			_, _ = fmt.Fprintf(h, "NotAfterStart:%d", activeLog.NotAfterStart.Unix())
		}
		if activeLog.NotAfterLimit != nil {
			_, _ = fmt.Fprintf(h, ":NotAfterLimit:%d", activeLog.NotAfterLimit.Unix())
		}
		annotations[labels.LabelNamespace+"/activeLogValidity"] = hex.EncodeToString(h.Sum(nil))
	}

	if len(instance.Spec.Logs) > 0 {
		h := sha256.New()
		hasNonActive := false
		for _, log := range instance.Spec.Logs {
			if log.Active != nil && *log.Active {
				continue
			}
			hasNonActive = true
			readonly := log.Readonly != nil && *log.Readonly
			publicKeyRefStr := ""
			privateKeyRefStr := ""
			privateKeyPasswordRefStr := ""
			if log.Signer != nil && log.Signer.File != nil {
				if log.Signer.File.PublicKeyRef != nil {
					publicKeyRefStr = log.Signer.File.PublicKeyRef.Name + "/" + log.Signer.File.PublicKeyRef.Key
				}
				if log.Signer.File.PrivateKeyRef != nil {
					privateKeyRefStr = log.Signer.File.PrivateKeyRef.Name + "/" + log.Signer.File.PrivateKeyRef.Key
				}
				if log.Signer.File.PrivateKeyPasswordRef != nil {
					privateKeyPasswordRefStr = log.Signer.File.PrivateKeyPasswordRef.Name + "/" + log.Signer.File.PrivateKeyPasswordRef.Key
				}
			}
			tokenLabelStr := ""
			if log.Signer != nil && log.Signer.PKCS11 != nil {
				if log.Signer.PKCS11.PublicKeyRef != nil {
					publicKeyRefStr = log.Signer.PKCS11.PublicKeyRef.Name + "/" + log.Signer.PKCS11.PublicKeyRef.Key
				}
				if log.Signer.PKCS11.PinSecretRef != nil {
					privateKeyPasswordRefStr = log.Signer.PKCS11.PinSecretRef.Name + "/" + log.Signer.PKCS11.PinSecretRef.Key
				}
				tokenLabelStr = log.Signer.PKCS11.TokenLabel
			}
			frozenSTHRefStr := ""
			if log.FrozenSTH != nil && log.FrozenSTH.TreeSize != nil {
				frozenSTHRefStr = fmt.Sprintf("%d", *log.FrozenSTH.TreeSize)
			}
			logId := int64(0)
			if log.LogId != nil {
				logId = *log.LogId
			}
			rootCertsStr := ""
			if len(log.RootCerts) > 0 {
				for _, r := range log.RootCerts {
					rootCertsStr += r.Name + "/" + r.Key + ","
				}
			}
			_, _ = fmt.Fprintf(h, "%d:%s:%s:%s:%s:%s:%s:%v", logId, publicKeyRefStr, privateKeyRefStr, privateKeyPasswordRefStr, tokenLabelStr, frozenSTHRefStr, rootCertsStr, readonly)
		}
		if hasNonActive {
			annotations[labels.LabelNamespace+"/shardingHash"] = hex.EncodeToString(h.Sum(nil))
		}
	}

	signerType := getSignerType(instance)
	if signerType == rhtasv1.SignerTypePKCS11 && activeLog != nil && activeLog.Signer != nil && activeLog.Signer.PKCS11 != nil {
		annotations[labels.LabelNamespace+"/pkcs11SpecHash"] = pkcs11SpecHash(activeLog.Signer.PKCS11)
	}

	return annotations
}

func getSignerType(instance *rhtasv1.CTlog) string {
	activeLog := ctlogUtils.ActiveLog(instance.Spec.Logs)
	if activeLog == nil || activeLog.Signer == nil {
		return rhtasv1.SignerTypeFile
	}
	return activeLog.Signer.Type
}
