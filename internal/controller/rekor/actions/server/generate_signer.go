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

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const secretNameFormat = "rekor-signer-%s-"

func NewGenerateSignerAction() action.Action[*v1alpha1.Rekor] {
	return &generateSigner{}
}

type generateSigner struct {
	action.BaseAction
}

func (g generateSigner) Name() string {
	return "generate-signer"
}

func (g generateSigner) CanHandle(_ context.Context, instance *v1alpha1.Rekor) bool {
	if !meta.IsStatusConditionTrue(instance.Status.Conditions, actions.SignerCondition) {
		return true
	}

	switch instance.Spec.Signer.KMS {
	case "secret", "":
		return instance.Status.Signer.KeyRef == nil || !equality.Semantic.DeepDerivative(instance.Spec.Signer, instance.Status.Signer)
	default:
		return !equality.Semantic.DeepDerivative(instance.Spec.Signer, instance.Status.Signer)
	}
}

func (g generateSigner) Handle(ctx context.Context, instance *v1alpha1.Rekor) *action.Result {
	if instance.Spec.Signer.KMS != "secret" && instance.Spec.Signer.KMS != "" {
		instance.Status.Signer = instance.Spec.Signer
		// force recreation of public key ref
		instance.Status.PublicKeyRef = nil
		// skip signer resolution and move to creating
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Creating,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionTrue,
			Reason:  constants.Ready,
			Message: "Not using Secret resource",
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   actions.ServerCondition,
			Status: metav1.ConditionFalse,
			Reason: constants.Creating,
		})
		return g.StatusUpdate(ctx, instance)
	}

	// Return to pending state because Signer spec changed
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Pending,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.SignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "resolving keys",
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    actions.ServerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "resolving keys",
		})
		return g.StatusUpdate(ctx, instance)
	}

	newSigner := *instance.Spec.Signer.DeepCopy()

	if instance.Spec.Signer.KeyRef == nil {
		privateKey, publicKey, err := g.createSignerKey()
		if err != nil {
			if !meta.IsStatusConditionFalse(instance.Status.Conditions, actions.SignerCondition) {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:    actions.SignerCondition,
					Status:  metav1.ConditionFalse,
					Reason:  constants.Failure,
					Message: err.Error(),
				})
				return g.StatusUpdate(ctx, instance)
			}
			// swallow error and retry
			return g.Requeue()
		}

		labels := constants.LabelsFor(actions.ServerComponentName, actions.ServerDeploymentName, instance.Name)

		data := map[string][]byte{
			"private": privateKey,
			"public":  publicKey,
		}
		secret := k8sutils.CreateImmutableSecret(fmt.Sprintf(secretNameFormat, instance.Name), instance.Namespace,
			data, labels)
		if _, err = g.Ensure(ctx, secret); err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    actions.SignerCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			return g.FailedWithStatusUpdate(ctx, fmt.Errorf("could not create secret: %w", err), instance)
		}
		g.Recorder.Eventf(instance, v1.EventTypeNormal, "SignerKeyCreated", "Signer private key created: %s", secret.Name)
		newSigner.KeyRef = &v1alpha1.SecretKeySelector{
			Key: "private",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}
	instance.Status.Signer = newSigner
	// force recreation of public key ref
	instance.Status.PublicKeyRef = nil

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   actions.ServerCondition,
		Status: metav1.ConditionFalse,
		Reason: constants.Creating,
	})
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   actions.SignerCondition,
		Status: metav1.ConditionTrue,
		Reason: constants.Ready,
	})
	return g.StatusUpdate(ctx, instance)
}

func (g generateSigner) createSignerKey() ([]byte, []byte, error) {
	var err error

	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
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
