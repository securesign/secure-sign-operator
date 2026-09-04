package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/generateSigner"
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
			ResolveRef:   resolveRef,
			GenerateData: generateData,
			AlignStatus:  alignStatus,
			IsEnabled: func(i *rhtasv1.CTlog) bool {
				activeLog := utils.ActiveLog(i.Spec.Logs)
				if activeLog == nil || activeLog.Signer == nil {
					return true
				}
				return activeLog.Signer.Type == rhtasv1.SignerTypeFile || activeLog.Signer.Type == ""
			},
			MutateSecret: func(_ *rhtasv1.CTlog, secret *corev1.Secret) {
				if secret.Labels == nil {
					secret.Labels = make(map[string]string)
				}
				secret.Labels[labels.LabelNamespace+"/ctfe.pub"] = constants.KeyPublic
			},
		}),
	)
}

func resolveRef(ctx context.Context, instance *rhtasv1.CTlog, c client.Client) (*rhtasv1.SecretKeySelector, error) {
	activeLog := utils.ActiveLog(instance.Spec.Logs)
	if activeLog != nil && activeLog.Signer != nil && activeLog.Signer.File != nil && activeLog.Signer.File.PrivateKeyRef != nil {
		ref := activeLog.Signer.File.PrivateKeyRef
		if err := generateSigner.RequireSecret(ctx, c, instance.Namespace, ref); err != nil {
			return nil, err
		}
		return ref, nil
	}
	activeLogStatus := utils.ActiveLogStatus(instance.Status.Logs)
	if activeLogStatus == nil || activeLogStatus.PrivateKeyRef == nil {
		return nil, nil
	}
	return generateSigner.ResolveStatusSecret(ctx, c, activeLogStatus.PrivateKeyRef, instance.Namespace, fmt.Sprintf(signerSecretNameFormat, instance.Name))
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

func alignStatus(instance *rhtasv1.CTlog, ref rhtasv1.SecretKeySelector) {
	activeLog := utils.ActiveLog(instance.Spec.Logs)
	if activeLog == nil {
		return
	}

	activeLogStatus := utils.ActiveLogStatus(instance.Status.Logs)
	if activeLogStatus == nil {
		return
	}

	var file *rhtasv1.CTlogFile
	if activeLog.Signer != nil {
		file = activeLog.Signer.File
	}

	if file != nil && file.PrivateKeyRef != nil {
		activeLogStatus.PrivateKeyRef = file.PrivateKeyRef

		if file.PublicKeyRef != nil {
			activeLogStatus.PublicKeyRef = file.PublicKeyRef
		} else {
			activeLogStatus.PublicKeyRef = &rhtasv1.SecretKeySelector{
				LocalObjectReference: file.PrivateKeyRef.LocalObjectReference,
				Key:                  constants.KeyPublic,
			}
		}
	} else {
		activeLogStatus.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyPrivate,
			LocalObjectReference: ref.LocalObjectReference,
		}
		activeLogStatus.PublicKeyRef = &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyPublic,
			LocalObjectReference: ref.LocalObjectReference,
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ConfigCondition,
		Status:  metav1.ConditionFalse,
		Reason:  SignerKeyReason,
		Message: "New signer key",
	})
}
