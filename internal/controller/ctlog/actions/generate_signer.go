package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	generateSigner "github.com/securesign/operator/internal/action/generateSigner"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/labels"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	signerSecretNameFormat = "ctlog-keys-config-%s"
)

func NewGenerateSignerAction() action.Action[*rhtasv1.CTlog] {
	return generateSigner.NewAction(
		SignerCondition,
		signerSecretNameFormat,
		ComponentName,
		DeploymentName,
		generateSigner.Wrapper(generateSigner.Config[*rhtasv1.CTlog]{
			Resolve:      resolve,
			GenerateData: generateData,
			AlignStatus:  alignStatus,
			MutateSecret: func(_ *rhtasv1.CTlog, secret *corev1.Secret) {
				if secret.Labels == nil {
					secret.Labels = make(map[string]string)
				}
				secret.Labels[labels.LabelNamespace+"/ctfe.pub"] = constants.KeyPublic
			},
		}),
	)
}

func resolve(ctx context.Context, instance *rhtasv1.CTlog, c client.Client) bool {
	if instance.Spec.PrivateKeyRef != nil {
		instance.Status.PrivateKeyRef = instance.Spec.PrivateKeyRef
		instance.Status.PrivateKeyPasswordRef = instance.Spec.PrivateKeyPasswordRef
		if instance.Spec.PublicKeyRef != nil {
			instance.Status.PublicKeyRef = instance.Spec.PublicKeyRef
		} else {
			instance.Status.PublicKeyRef = &rhtasv1.SecretKeySelector{
				LocalObjectReference: instance.Spec.PrivateKeyRef.LocalObjectReference,
				Key:                  constants.KeyPublic,
			}
		}

		// invalidate server config
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    ConfigCondition,
			Status:  metav1.ConditionFalse,
			Reason:  SignerKeyReason,
			Message: "New signer key",
		})
		return true
	}
	// Upgrade from <1.5.0: check if status references an old GenerateName-based secret
	if instance.Status.PrivateKeyRef != nil {
		name := instance.Status.PrivateKeyRef.Name
		if name != "" && name != fmt.Sprintf(signerSecretNameFormat, instance.Name) {
			existing := &corev1.Secret{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: instance.Namespace},
			}), existing); err == nil {
				// Reuse old secret — keep status pointing to the pre-existing secret
				return true
			}
		}
	}
	return false
}

func generateData(_ context.Context, _ *rhtasv1.CTlog, _ client.Client) (map[string][]byte, error) {
	keyConfig, err := utils.CreatePrivateKey()
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		constants.KeyPrivate: keyConfig.PrivateKey,
		constants.KeyPublic:  keyConfig.PublicKey,
	}, nil
}

func alignStatus(instance *rhtasv1.CTlog, secret *corev1.Secret) {
	instance.Status.PrivateKeyRef = &rhtasv1.SecretKeySelector{
		Key: constants.KeyPrivate,
		LocalObjectReference: rhtasv1.LocalObjectReference{
			Name: secret.Name,
		},
	}
	instance.Status.PublicKeyRef = &rhtasv1.SecretKeySelector{
		Key: constants.KeyPublic,
		LocalObjectReference: rhtasv1.LocalObjectReference{
			Name: secret.Name,
		},
	}

	// invalidate server config
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ConfigCondition,
		Status:  metav1.ConditionFalse,
		Reason:  SignerKeyReason,
		Message: "New signer key",
	})
}
