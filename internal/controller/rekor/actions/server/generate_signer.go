package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"slices"
	"time"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/state"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	secretNameFormat = "rekor-signer-%s-"
	RekorSignerLabel = labels.LabelNamespace + "/rekor.signer.pem"
	signerKMSSecret  = "secret"
)

func NewGenerateSignerAction() action.Action[*rhtasv1.Rekor] {
	return &generateSigner{}
}

type generateSigner struct {
	action.BaseAction
}

func (g generateSigner) Name() string {
	return "generate-signer"
}

func (g generateSigner) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	// SignerCondition is managed exclusively by this action.
	c := meta.FindStatusCondition(instance.Status.Conditions, actions.SignerCondition)
	return c == nil || c.Status != metav1.ConditionTrue || c.ObservedGeneration != instance.Generation
}

func (g generateSigner) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	if instance.Spec.Signer.KMS != signerKMSSecret && instance.Spec.Signer.KMS != "" {
		if instance.Status.Signer.KeyRef == nil && instance.Status.Signer.PasswordRef == nil {
			// KMS mode already configured — re-stamp generation
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               actions.SignerCondition,
				Status:             metav1.ConditionTrue,
				Reason:             state.Ready.String(),
				Message:            "Not using Secret resource",
				ObservedGeneration: instance.Generation,
			})
			return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
		}

		// Transitioning from secret to KMS — clear stale key references
		instance.Status.Signer = rhtasv1.RekorSignerStatus{}
		instance.Status.PublicKeyRef = nil
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Creating.String(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               actions.SignerCondition,
			Status:             metav1.ConditionTrue,
			Reason:             state.Ready.String(),
			Message:            "Not using Secret resource",
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   actions.ServerCondition,
			Status: metav1.ConditionFalse,
			Reason: state.Creating.String(),
		})
		return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
	}

	// Secret mode — check if signer config actually changed
	if instance.Status.Signer.KeyRef != nil &&
		equality.Semantic.DeepDerivative(instance.Spec.Signer.KeyRef, instance.Status.Signer.KeyRef) &&
		equality.Semantic.DeepDerivative(instance.Spec.Signer.PasswordRef, instance.Status.Signer.PasswordRef) { //nolint:staticcheck
		// Signer unchanged — re-stamp generation
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               actions.SignerCondition,
			Status:             metav1.ConditionTrue,
			Reason:             state.Ready.String(),
			ObservedGeneration: instance.Generation,
		})
		return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
	}

	newSignerStatus := rhtasv1.RekorSignerStatus{
		KeyRef:      instance.Spec.Signer.KeyRef,
		PasswordRef: instance.Spec.Signer.PasswordRef, //nolint:staticcheck
	}

	// Return to pending state because Signer spec changed
	if state.FromInstance(instance, constants.ReadyCondition) != state.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Pending.String(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               actions.SignerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             state.Pending.String(),
			Message:            "resolving keys",
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  state.Pending.String(),
			Message: "resolving keys",
		})
		return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
	}

	if instance.Spec.Signer.KeyRef == nil {

		partialSecret, err := kubernetes.FindSecret(ctx, g.Client, instance.Namespace, RekorSignerLabel)
		if err != nil && !apierrors.IsNotFound(err) {
			g.Logger.Error(err, "problem with finding secret", "namespace", instance.Namespace)
		}
		if partialSecret != nil {
			newSignerStatus.KeyRef = &rhtasv1.SecretKeySelector{
				Key: constants.KeyPrivate,
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: partialSecret.Name,
				},
			}
		} else {
			componentLabels := labels.For(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)
			signerLabels := map[string]string{RekorSignerLabel: constants.KeyPrivate}
			privateKey, publicKey, err := g.createSignerKey()
			if err != nil {
				if !meta.IsStatusConditionFalse(instance.Status.Conditions, actions.SignerCondition) {
					meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
						Type:               actions.SignerCondition,
						Status:             metav1.ConditionFalse,
						Reason:             state.Failure.String(),
						Message:            err.Error(),
						ObservedGeneration: instance.Generation,
					})
					return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
				}
				// swallow error and retry
				return g.RequeueAfter(5 * time.Second)
			}

			data := map[string][]byte{
				constants.KeyPrivate: privateKey,
				constants.KeyPublic:  publicKey,
			}

			secret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: fmt.Sprintf(secretNameFormat, instance.Name),
					Namespace:    instance.Namespace,
				},
			}

			if _, err = kubernetes.CreateOrUpdate(ctx, g.Client,
				secret,
				ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(componentLabels)), componentLabels),
				ensure.Labels[*v1.Secret](slices.Collect(maps.Keys(signerLabels)), signerLabels),
				kubernetes.EnsureSecretData(true, data),
			); err != nil {
				return g.Error(ctx, fmt.Errorf("could not create signer secret: %w", err), instance,
					metav1.Condition{
						Type:               actions.SignerCondition,
						Status:             metav1.ConditionFalse,
						Reason:             state.Failure.String(),
						Message:            err.Error(),
						ObservedGeneration: instance.Generation,
					})
			}

			g.Recorder.Eventf(instance, secret, v1.EventTypeNormal, "SignerKeyCreated", "Created", "Signer private key created: %s", secret.Name)
			newSignerStatus.KeyRef = &rhtasv1.SecretKeySelector{
				Key: constants.KeyPrivate,
				LocalObjectReference: rhtasv1.LocalObjectReference{
					Name: secret.Name,
				},
			}
		}
	}
	instance.Status.Signer = newSignerStatus
	// force recreation of public key ref
	instance.Status.PublicKeyRef = nil

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   actions.ServerCondition,
		Status: metav1.ConditionFalse,
		Reason: state.Creating.String(),
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               actions.SignerCondition,
		Status:             metav1.ConditionTrue,
		Reason:             state.Ready.String(),
		ObservedGeneration: instance.Generation,
	})
	return g.ReturnOnChange(g.PersistStatus)(ctx, instance)
}

func (g generateSigner) createSignerKey() ([]byte, []byte, error) {
	var err error

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}

	mPubKey, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, nil, err
	}

	var pemRekorKey bytes.Buffer
	err = pem.Encode(&pemRekorKey, &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: mKey,
	})
	if err != nil {
		return nil, nil, err
	}

	var pemPubKey bytes.Buffer
	err = pem.Encode(&pemPubKey, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: mPubKey,
	})
	if err != nil {
		return nil, nil, err
	}

	return pemRekorKey.Bytes(), pemPubKey.Bytes(), nil
}
