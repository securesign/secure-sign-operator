package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KeySecretName = "ctlog-keys-"
)

func NewHandleKeysAction() action.Action[*v1alpha1.CTlog] {
	return &handleKeys{}
}

type handleKeys struct {
	action.BaseAction
}

func (g handleKeys) Name() string {
	return "handle-keys"
}

func (g handleKeys) CanHandle(ctx context.Context, instance *v1alpha1.CTlog) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)

	switch {
	case c == nil:
		return false
	case c.Reason != constants.Creating && c.Reason != constants.Ready:
		return false
	case instance.Status.PrivateKeyRef == nil || instance.Status.PublicKeyRef == nil:
		return true
	case !equality.Semantic.DeepDerivative(instance.Spec.PrivateKeyRef, instance.Status.PrivateKeyRef):
		return true
	case !equality.Semantic.DeepDerivative(instance.Spec.PublicKeyRef, instance.Status.PublicKeyRef):
		return true
	case !equality.Semantic.DeepDerivative(instance.Spec.PrivateKeyPasswordRef, instance.Status.PrivateKeyPasswordRef):
		return true
	}
	return false
}

func (g handleKeys) Handle(ctx context.Context, instance *v1alpha1.CTlog) *action.Result {
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Creating {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			ObservedGeneration: instance.Generation,
		},
		)
		return g.StatusUpdate(ctx, instance)
	}

	newKeyStatus := instance.Status.DeepCopy()
	if instance.Spec.PrivateKeyPasswordRef != nil {
		newKeyStatus.PrivateKeyPasswordRef = instance.Spec.PrivateKeyPasswordRef
	}

	g.discoverPrivateKey(ctx, instance, newKeyStatus)
	g.discoverPubliceKey(ctx, instance, newKeyStatus)

	keys, err := g.setupKeys(instance.Namespace, newKeyStatus)
	if err != nil {
		return g.Error(ctx, fmt.Errorf("could not generate keys: %w", err), instance)
	}
	if _, err = g.generateAndUploadSecret(ctx, instance, newKeyStatus, keys); err != nil {
		return g.Error(ctx, fmt.Errorf("could not generate Secret: %w", err), instance)
	}

	instance.Status = *newKeyStatus

	// invalidate server config
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ConfigCondition,
		Status:  metav1.ConditionFalse,
		Reason:  SignerKeyReason,
		Message: "New signer key",
	})

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               constants.Ready,
		Status:             metav1.ConditionFalse,
		Reason:             constants.Creating,
		Message:            "Keys resolved",
		ObservedGeneration: instance.Generation,
	})
	return g.StatusUpdate(ctx, instance)
}

func (g handleKeys) setupKeys(ns string, instanceStatus *v1alpha1.CTlogStatus) (*utils.KeyConfig, error) {
	var (
		err    error
		config = &utils.KeyConfig{}
	)
	if instanceStatus.PrivateKeyPasswordRef != nil {
		config.PrivateKeyPass, err = kubernetes.GetSecretData(g.Client, ns, instanceStatus.PrivateKeyPasswordRef)
		if err != nil {
			return nil, err
		}
	}

	if instanceStatus.PrivateKeyRef == nil {
		return utils.CreatePrivateKey(config.PrivateKeyPass)
	} else {

		config.PrivateKey, err = kubernetes.GetSecretData(g.Client, ns, instanceStatus.PrivateKeyRef)
		if err != nil {
			return nil, err
		}
	}

	if instanceStatus.PublicKeyRef == nil {
		return utils.GeneratePublicKey(config)
	} else {
		config.PublicKey, err = kubernetes.GetSecretData(g.Client, ns, instanceStatus.PrivateKeyRef)
		if err != nil {
			return nil, err
		}
		return config, nil
	}
}

func (g handleKeys) discoverPrivateKey(ctx context.Context, instance *v1alpha1.CTlog, newKeyStatus *v1alpha1.CTlogStatus) {
	if instance.Spec.PrivateKeyRef != nil {
		newKeyStatus.PrivateKeyRef = instance.Spec.PrivateKeyRef
		return
	}

	partialPrivateSecrets, err := kubernetes.ListSecrets(ctx, g.Client, instance.Namespace, CTLogPrivateLabel)
	if err != nil {
		g.Logger.Error(err, "problem with listing secrets", "namespace", instance.Namespace)
	}
	for _, partialPrivateSecret := range partialPrivateSecrets.Items {
		if newKeyStatus.PrivateKeyRef == nil {
			// we are still searching for new key
			passwordKeyRef, isEncrypted := partialPrivateSecret.Annotations[passwordKeyRefAnnotation]
			if newKeyStatus.PrivateKeyPasswordRef != nil {
				// we search for password encrypted private key
				if isEncrypted && newKeyStatus.PrivateKeyPasswordRef.Name == passwordKeyRef {
					g.Recorder.Event(instance, v1.EventTypeNormal, "PrivateKeyDiscovered", "Existing private key discovered")
					newKeyStatus.PrivateKeyRef = g.sksByLabel(partialPrivateSecret, CTLogPrivateLabel)
					continue
				}
			} else if !isEncrypted {
				g.Recorder.Event(instance, v1.EventTypeNormal, "PrivateKeyDiscovered", "Existing private key discovered")
				newKeyStatus.PrivateKeyRef = g.sksByLabel(partialPrivateSecret, CTLogPrivateLabel)
				continue
			}
		}
		err = labels.Remove(ctx, &partialPrivateSecret, g.Client, CTLogPrivateLabel)
		if err != nil {
			g.Logger.Error(err, "problem with invalidating private key secret", "namespace", instance.Namespace)
		}
		g.Recorder.Event(instance, v1.EventTypeNormal, "PrivateSecretLabelRemoved", "Private key secret invalidated")
	}
}

func (g handleKeys) discoverPubliceKey(ctx context.Context, instance *v1alpha1.CTlog, newKeyStatus *v1alpha1.CTlogStatus) {
	switch {
	case instance.Spec.PublicKeyRef != nil:
		newKeyStatus.PublicKeyRef = instance.Spec.PublicKeyRef
		return
	case newKeyStatus.PrivateKeyRef == nil:
		// need to generate new pair
		return
	}

	partialPubSecrets, err := kubernetes.ListSecrets(ctx, g.Client, instance.Namespace, CTLPubLabel)
	if err != nil {
		g.Logger.Error(err, "problem with listing secrets", "namespace", instance.Namespace)
	}
	for _, partialPubSecret := range partialPubSecrets.Items {
		if newKeyStatus.PublicKeyRef == nil {
			// we are still searching for new key
			if privateKeyRef, ok := partialPubSecret.Annotations[privateKeyRefAnnotation]; ok && privateKeyRef == newKeyStatus.PrivateKeyRef.Name {
				g.Recorder.Event(instance, v1.EventTypeNormal, "PublicKeyDiscovered", "Existing public key discovered")
				newKeyStatus.PublicKeyRef = g.sksByLabel(partialPubSecret, CTLPubLabel)
				continue
			}
		}
		err = labels.Remove(ctx, &partialPubSecret, g.Client, CTLPubLabel)
		if err != nil {
			g.Logger.Error(err, "problem with invalidating public key secret", "namespace", instance.Namespace)
		}
		g.Recorder.Event(instance, v1.EventTypeNormal, "PrivateSecretLabelRemoved", "Public key secret invalidated")
	}
}

func (g handleKeys) generateAndUploadSecret(ctx context.Context, instance *v1alpha1.CTlog, newKeyStatus *v1alpha1.CTlogStatus, keys *utils.KeyConfig) (*v1.Secret, error) {
	if newKeyStatus.PublicKeyRef != nil && newKeyStatus.PrivateKeyRef != nil {
		return nil, nil
	}

	var err error

	componentLabels := labels.For(ComponentName, DeploymentName, instance.Name)
	keyRelatedLabels := make(map[string]string)
	annotations := make(map[string]string)
	data := make(map[string][]byte)

	if newKeyStatus.PrivateKeyRef == nil {
		if newKeyStatus.PrivateKeyPasswordRef != nil {
			annotations[passwordKeyRefAnnotation] = newKeyStatus.PrivateKeyPasswordRef.Name
		}
		data["private"] = keys.PrivateKey
		keyRelatedLabels[CTLogPrivateLabel] = "private"
	}

	if newKeyStatus.PublicKeyRef == nil {
		if newKeyStatus.PrivateKeyRef != nil {
			annotations[privateKeyRefAnnotation] = newKeyStatus.PrivateKeyRef.Name
		}
		data["public"] = keys.PublicKey
		keyRelatedLabels[CTLPubLabel] = "public"
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: KeySecretName,
			Namespace:    instance.Namespace,
		},
	}
	if _, err = kubernetes.CreateOrUpdate(ctx, g.Client,
		secret,
		ensure.ControllerReference[*v1.Secret](instance, g.Client),
		ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
		ensure.Labels[*v1.Secret](ManagedLabels, keyRelatedLabels),
		ensure.Annotations[*v1.Secret](ManagedAnnotations, annotations),
		kubernetes.EnsureSecretData(true, data),
	); err != nil {
		return nil, err
	}

	if _, ok := secret.Labels[CTLPubLabel]; ok {
		newKeyStatus.PublicKeyRef = &v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
			Key: "public",
		}
	}

	if _, ok := secret.Labels[CTLogPrivateLabel]; ok {
		newKeyStatus.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
			Key: "private",
		}
	}

	if _, ok := secret.Labels[CTLPubLabel]; ok {
		if _, ok = secret.Labels[CTLogPrivateLabel]; ok {
			// need to add cyclic private key reference annotation
			annotations[privateKeyRefAnnotation] = secret.Name
			if _, err = kubernetes.CreateOrUpdate(ctx, g.Client,
				secret,
				ensure.Annotations[*v1.Secret](ManagedAnnotations, annotations),
			); err != nil {
				return nil, err
			}
		}
	}
	return secret, nil
}

func (g handleKeys) sksByLabel(secret metav1.PartialObjectMetadata, label string) *v1alpha1.SecretKeySelector {
	return &v1alpha1.SecretKeySelector{
		Key: secret.Labels[label],
		LocalObjectReference: v1alpha1.LocalObjectReference{
			Name: secret.Name,
		},
	}
}
