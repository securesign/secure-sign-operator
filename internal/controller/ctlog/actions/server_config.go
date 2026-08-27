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
	"k8s.io/apimachinery/pkg/api/equality"
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
	case (instance.Spec.Signer.Type == rhtasv1.SignerTypeFile || instance.Spec.Signer.Type == "") && instance.Status.PrivateKeyRef == nil:
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

	isPKCS11 := instance.Spec.Signer.Type == rhtasv1.SignerTypePKCS11

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
		if instance.Spec.NotAfterStart != nil {
			notAfterStart = instance.Spec.NotAfterStart.Unix()
		}
		if instance.Spec.NotAfterLimit != nil {
			notAfterLimit = instance.Spec.NotAfterLimit.Unix()
		}
		cfg, err = ctlogUtils.CreateCtlogConfig(trillianUrl, *instance.Status.TreeID, rootCerts, certConfig, instance.Spec.Prefix, shards, notAfterStart, notAfterLimit)
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
	p := instance.Spec.Signer.PKCS11
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

	return ctlogUtils.CreateCtlogPKCS11Config(
		trillianUrl,
		*instance.Status.TreeID,
		rootCerts,
		p.TokenLabel,
		string(pin),
		publicKey,
		instance.Spec.Prefix,
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
	password, err := kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, instance.Status.PrivateKeyPasswordRef)
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
	shards := make([]ctlogUtils.ShardConfig, 0, len(instance.Spec.Sharding))
	for _, s := range instance.Spec.Sharding {
		var publicKey []byte
		if s.PublicKeyRef != nil {
			var err error
			publicKey, err = kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, s.PublicKeyRef)
			if err != nil {
				return nil, fmt.Errorf("shard %d publicKeyRef: %w", s.TreeID, err)
			}
		}

		var frozenSTH []byte
		if s.FrozenSTHRef != nil {
			var err error
			frozenSTH, err = kubernetes.GetSecretData(ctx, i.Client, instance.Namespace, s.FrozenSTHRef)
			if err != nil {
				return nil, fmt.Errorf("shard %d frozenSTHRef: %w", s.TreeID, err)
			}
		}

		sc := ctlogUtils.ShardConfig{
			TreeID:    s.TreeID,
			PublicKey: publicKey,
			Prefix:    s.Prefix,
			FrozenSTH: frozenSTH,
		}

		// Set validity timestamps if provided
		if s.NotAfterStart != nil {
			sc.NotAfterStart = s.NotAfterStart.Unix()
		}
		if s.NotAfterLimit != nil {
			sc.NotAfterLimit = s.NotAfterLimit.Unix()
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

	// Check if the secret was generated from the same data sources using annotations
	expectedAnnotations := i.configMatchingAnnotations(ctx, instance, trillianUrl)
	if !equality.Semantic.DeepDerivative(expectedAnnotations, secretMeta.GetAnnotations()) {
		return errSecretInvalid
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

	if instance.Spec.Prefix != "" {
		annotations[labels.LabelNamespace+"/logPrefix"] = instance.Spec.Prefix
	}

	if instance.Spec.NotAfterStart != nil || instance.Spec.NotAfterLimit != nil {
		h := sha256.New()
		if instance.Spec.NotAfterStart != nil {
			_, _ = fmt.Fprintf(h, "NotAfterStart:%d", instance.Spec.NotAfterStart.Unix())
		}
		if instance.Spec.NotAfterLimit != nil {
			_, _ = fmt.Fprintf(h, ":NotAfterLimit:%d", instance.Spec.NotAfterLimit.Unix())
		}
		annotations[labels.LabelNamespace+"/activeLogValidity"] = hex.EncodeToString(h.Sum(nil))
	}

	if len(instance.Spec.Sharding) > 0 {
		h := sha256.New()
		for _, shard := range instance.Spec.Sharding {
			publicKeyRefStr := ""
			if shard.PublicKeyRef != nil {
				publicKeyRefStr = shard.PublicKeyRef.Name + "/" + shard.PublicKeyRef.Key
			}
			frozenSTHRefStr := ""
			if shard.FrozenSTHRef != nil {
				frozenSTHRefStr = shard.FrozenSTHRef.Name + "/" + shard.FrozenSTHRef.Key
			}
			_, _ = fmt.Fprintf(h, "%d:%s:%s", shard.TreeID, publicKeyRefStr, frozenSTHRefStr)
		}
		annotations[labels.LabelNamespace+"/shardingHash"] = hex.EncodeToString(h.Sum(nil))
	}

	if instance.Spec.Signer.Type == rhtasv1.SignerTypePKCS11 && instance.Spec.Signer.PKCS11 != nil {
		annotations[labels.LabelNamespace+"/pkcs11SpecHash"] = pkcs11SpecHash(instance.Spec.Signer.PKCS11)
	}

	return annotations
}
