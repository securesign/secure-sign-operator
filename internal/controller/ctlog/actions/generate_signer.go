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
	"k8s.io/apimachinery/pkg/api/equality"
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
	return generateSigner.ResolveStatusSecret(ctx, c, instance.Status.PrivateKeyRef, instance.Namespace, fmt.Sprintf(signerSecretNameFormat, instance.Name))
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
	oldPasswordRef := instance.Status.PrivateKeyPasswordRef
	oldPrivateKeyRef := instance.Status.PrivateKeyRef

	var file *rhtasv1.CTlogFile
	activeLog := utils.ActiveLog(instance.Spec.Logs)
	if activeLog != nil && activeLog.Signer != nil {
		file = activeLog.Signer.File
	}
	if file != nil && file.PrivateKeyRef != nil {
		instance.Status.PrivateKeyRef = file.PrivateKeyRef

		//TODO: Status.PublicKey resolver will be extracted to separate action.
		if file.PublicKeyRef != nil {
			instance.Status.PublicKeyRef = file.PublicKeyRef
		} else {
			instance.Status.PublicKeyRef = &rhtasv1.SecretKeySelector{
				LocalObjectReference: file.PrivateKeyRef.LocalObjectReference,
				Key:                  constants.KeyPublic,
			}
		}
	} else {
		instance.Status.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyPrivate,
			LocalObjectReference: ref.LocalObjectReference,
		}
		instance.Status.PublicKeyRef = &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyPublic,
			LocalObjectReference: ref.LocalObjectReference,
		}
	}

	if oldPasswordRef != nil &&
		equality.Semantic.DeepEqual(instance.Status.PrivateKeyRef, oldPrivateKeyRef) {
		instance.Status.PrivateKeyPasswordRef = oldPasswordRef
	} else {
		instance.Status.PrivateKeyPasswordRef = nil
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:    ConfigCondition,
		Status:  metav1.ConditionFalse,
		Reason:  SignerKeyReason,
		Message: "New signer key",
	})
}
