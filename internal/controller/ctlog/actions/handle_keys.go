package actions

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
		return g.Failed(err)
	}
	if _, err = g.generateAndUploadSecret(ctx, instance, newKeyStatus, keys); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return g.FailedWithStatusUpdate(ctx, fmt.Errorf("could not update Secret annotations: %w", err), instance)
	}

	instance.Status = *newKeyStatus

	// invalidate server config
	if instance.Status.ServerConfigRef != nil {
		instance.Status.ServerConfigRef = nil
	}

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
		config.PrivateKeyPass, err = k8sutils.GetSecretData(g.Client, ns, instanceStatus.PrivateKeyPasswordRef)
		if err != nil {
			return nil, err
		}
	}

	if instanceStatus.PrivateKeyRef == nil {
		return utils.CreatePrivateKey(config.PrivateKeyPass)
	} else {

		config.PrivateKey, err = k8sutils.GetSecretData(g.Client, ns, instanceStatus.PrivateKeyRef)
		if err != nil {
			return nil, err
		}
	}

	if instanceStatus.PublicKeyRef == nil {
		return utils.GeneratePublicKey(config)
	} else {
		config.PublicKey, err = k8sutils.GetSecretData(g.Client, ns, instanceStatus.PrivateKeyRef)
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

	partialPrivateSecrets, err := k8sutils.ListSecrets(ctx, g.Client, instance.Namespace, CTLogPrivateLabel)
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
		err = constants.RemoveLabel(ctx, &partialPrivateSecret, g.Client, CTLogPrivateLabel)
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

	partialPubSecrets, err := k8sutils.ListSecrets(ctx, g.Client, instance.Namespace, CTLPubLabel)
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
		err = constants.RemoveLabel(ctx, &partialPubSecret, g.Client, CTLPubLabel)
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

	secret := k8sutils.CreateImmutableSecret(KeySecretName, instance.Namespace, map[string][]byte{}, constants.LabelsFor(ComponentName, DeploymentName, instance.Name))
	secret.Annotations = make(map[string]string)

	if newKeyStatus.PrivateKeyRef == nil {
		if newKeyStatus.PrivateKeyPasswordRef != nil {
			secret.Annotations[passwordKeyRefAnnotation] = newKeyStatus.PrivateKeyPasswordRef.Name
		}
		secret.Data["private"] = keys.PrivateKey
		secret.Labels[CTLogPrivateLabel] = "private"
	}

	if newKeyStatus.PublicKeyRef == nil {
		if newKeyStatus.PrivateKeyRef != nil {
			secret.Annotations[privateKeyRefAnnotation] = newKeyStatus.PrivateKeyRef.Name
		}
		secret.Data["public"] = keys.PublicKey
		secret.Labels[CTLPubLabel] = "public"
	}

	if err := controllerutil.SetControllerReference(instance, secret, g.Client.Scheme()); err != nil {
		return nil, err
	}

	// we need to upload secret to get secretName
	if _, err := g.Ensure(ctx, secret); err != nil {
		return nil, err
	}
	// refetch secret to avoid "resourceVersion should not be set on objects to be created"
	time.Sleep(time.Second)
	if err := g.Client.Get(ctx, client.ObjectKeyFromObject(secret), secret); err != nil {
		return nil, err
	}

	if slices.Contains(maps.Keys(secret.Labels), CTLPubLabel) {
		newKeyStatus.PublicKeyRef = &v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
			Key: "public",
		}
	}

	if slices.Contains(maps.Keys(secret.Labels), CTLogPrivateLabel) {
		newKeyStatus.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
			Key: "private",
		}
	}

	if slices.Contains(maps.Keys(secret.Labels), CTLPubLabel) && slices.Contains(maps.Keys(secret.Labels), CTLogPrivateLabel) {
		// need to add cyclic private key reference annotation
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}
		secret.Annotations[privateKeyRefAnnotation] = secret.Name
		if _, err := g.Ensure(ctx, secret, action.EnsureAnnotations(privateKeyRefAnnotation)); err != nil {
			return nil, err
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
