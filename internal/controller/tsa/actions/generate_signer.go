package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"maps"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common"
	"github.com/securesign/operator/internal/controller/common/action"
	"github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TSACertCALabel = constants.LabelNamespace + "/tsa.certchain.pem"
)

type generateSigner struct {
	action.BaseAction
}

func NewGenerateSignerAction() action.Action[*v1alpha1.TimestampAuthority] {
	return &generateSigner{}
}

func (g generateSigner) Name() string {
	return "handle certificate chain"
}

func (g generateSigner) CanHandle(_ context.Context, instance *v1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.GetConditions(), constants.Ready)
	return (c.Reason == constants.Pending || c.Reason == constants.Ready) && (instance.Status.Signer == nil ||
		!equality.Semantic.DeepDerivative(instance.Spec.Signer, *instance.Status.Signer))
}

func (g generateSigner) Handle(ctx context.Context, instance *v1alpha1.TimestampAuthority) *action.Result {
	var (
		err error
	)

	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Pending,
		},
		)
		return g.StatusUpdate(ctx, instance)
	}

	tsaCertChainConfig := &tsaUtils.TsaCertChainConfig{}
	if tsaUtils.IsFileType(instance) {
		tsaCertChainConfig, err = g.handleSignerKeys(instance, tsaCertChainConfig)
		if err != nil {
			g.Logger.Error(err, "error resolving keys for timestamp authority")
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    TSASignerCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Pending,
				Message: "Resolving keys",
			})
			g.StatusUpdate(ctx, instance)
			// swallow error and retry
			return g.Requeue()
		}
	}

	tsaCertChainConfig, err = g.handleCertificateChain(ctx, instance, tsaCertChainConfig)
	if err != nil {
		g.Logger.Error(err, "error resolving certificate chain for timestamp authority")
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    TSASignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Pending,
			Message: "Resolving keys",
		})
		g.StatusUpdate(ctx, instance)
		// swallow error and retry
		return g.Requeue()
	}

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
	secretLabels := map[string]string{
		TSACertCALabel: "certificateChain",
	}
	maps.Copy(secretLabels, labels)

	certificateChain := k8sutils.CreateImmutableSecret(fmt.Sprintf("tsa-signer-config-%s", instance.Name), instance.Namespace, tsaCertChainConfig.ToMap(), secretLabels)
	//Check if a secret for the signer config already exists, if it does remove the label
	secret, err := kubernetes.FindSecret(ctx, g.Client, instance.Namespace, TSACertCALabel)
	if err != nil && !apierrors.IsNotFound(err) {
		g.Logger.Error(err, "problem with finding secret", "namespace", instance.Namespace)
	}

	if secret != nil {
		if err := constants.RemoveLabel(ctx, secret, g.Client, TSACertCALabel); err != nil {
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    TSASignerCondition,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:    constants.Ready,
				Status:  metav1.ConditionFalse,
				Reason:  constants.Failure,
				Message: err.Error(),
			})
			return g.FailedWithStatusUpdate(ctx, err, instance)
		}
		message := fmt.Sprintf("Removed '%s' label from %s secret", TSACertCALabel, secret.Name)
		g.Recorder.Event(instance, v1.EventTypeNormal, "CertificateChainSecretLabelRemoved", message)
		g.Logger.Info(message)
	}

	if _, err := g.Ensure(ctx, certificateChain); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    TSASignerCondition,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    constants.Ready,
			Status:  metav1.ConditionFalse,
			Reason:  constants.Failure,
			Message: err.Error(),
		})
		return g.FailedWithStatusUpdate(ctx, err, instance)
	}

	if instance.Status.Signer == nil {
		instance.Status.Signer = new(v1alpha1.TimestampAuthoritySigner)
	}
	if instance.Spec.Signer.File == nil && instance.Spec.Signer.CertificateChain.CertificateChainRef == nil {
		instance.Spec.Signer.File = new(v1alpha1.File)
	}
	instance.Spec.Signer.DeepCopyInto(instance.Status.Signer)

	if instance.Spec.Signer.CertificateChain.CertificateChainRef == nil {
		instance.Status.Signer.CertificateChain.CertificateChainRef = &v1alpha1.SecretKeySelector{
			Key: "certificateChain",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: certificateChain.Name,
			},
		}

		if instance.Spec.Signer.File.PrivateKeyRef == nil {
			instance.Status.Signer.File.PrivateKeyRef = &v1alpha1.SecretKeySelector{
				Key: "leafPrivateKey",
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: certificateChain.Name,
				},
			}
		}

		if instance.Spec.Signer.File.PasswordRef == nil && len(tsaCertChainConfig.LeafPrivateKeyPassword) > 0 {
			instance.Status.Signer.File.PasswordRef = &v1alpha1.SecretKeySelector{
				Key: "leafPrivateKeyPassword",
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: certificateChain.Name,
				},
			}
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   TSASignerCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	})
	return g.StatusUpdate(ctx, instance)
}

func (g generateSigner) handleSignerKeys(instance *v1alpha1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig) (*tsaUtils.TsaCertChainConfig, error) {
	if instance.Spec.Signer.File != nil {
		if instance.Spec.Signer.File.PrivateKeyRef != nil {
			key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, instance.Spec.Signer.File.PrivateKeyRef)
			if err != nil {
				return nil, err
			}
			config.LeafPrivateKey = key

			if ref := instance.Spec.Signer.File.PasswordRef; ref != nil {
				password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKeyPassword = password
			}
		}

		if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref == nil {
			config.RootPrivateKeyPassword = common.GeneratePassword(8)
			key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			if err != nil {
				return nil, err
			}
			rootCAPrivKey, err := tsaUtils.CreatePrivateKey(key, config.RootPrivateKeyPassword)
			if err != nil {
				return nil, err
			}
			config.RootPrivateKey = rootCAPrivKey
		}

	} else {
		if ref := instance.Spec.Signer.CertificateChain.RootCA.PrivateKeyRef; ref != nil {
			key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
			if err != nil {
				return nil, err
			}
			config.RootPrivateKey = key

			if ref := instance.Spec.Signer.CertificateChain.RootCA.PasswordRef; ref != nil {
				password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.RootPrivateKeyPassword = password
			}
		} else {
			config.RootPrivateKeyPassword = common.GeneratePassword(8)
			key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			if err != nil {
				return nil, err
			}
			rootCAPrivKey, err := tsaUtils.CreatePrivateKey(key, config.RootPrivateKeyPassword)
			if err != nil {
				return nil, err
			}
			config.RootPrivateKey = rootCAPrivKey
		}

		for index, intermediateCA := range instance.Spec.Signer.CertificateChain.IntermediateCA {
			if ref := intermediateCA.PrivateKeyRef; ref != nil {
				key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, key)

				if ref := intermediateCA.PasswordRef; ref != nil {
					password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, password)
				} else {
					config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, []byte(""))
				}
			} else {
				config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, common.GeneratePassword(8))
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				interCAPrivKey, err := tsaUtils.CreatePrivateKey(key, config.IntermediatePrivateKeyPasswords[index])
				if err != nil {
					return nil, err
				}
				config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, interCAPrivKey)
			}
		}

		if ref := instance.Spec.Signer.CertificateChain.LeafCA.PrivateKeyRef; ref != nil {
			key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
			if err != nil {
				return nil, err
			}
			config.LeafPrivateKey = key

			if ref := instance.Spec.Signer.CertificateChain.LeafCA.PasswordRef; ref != nil {
				password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKeyPassword = password
			}
		} else {
			config.LeafPrivateKeyPassword = common.GeneratePassword(8)
			key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			if err != nil {
				return nil, err
			}
			leafCAPrivKey, err := tsaUtils.CreatePrivateKey(key, config.LeafPrivateKeyPassword)
			if err != nil {
				return nil, err
			}
			config.LeafPrivateKey = leafCAPrivKey
		}

	}
	return config, nil
}

func (g generateSigner) handleCertificateChain(ctx context.Context, instance *v1alpha1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig) (*tsaUtils.TsaCertChainConfig, error) {
	if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref != nil {
		certificateChain, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.CertificateChain = certificateChain
	} else {
		certificateChain, err := tsaUtils.CreateTSACertChain(ctx, instance, DeploymentName, g.Client, config)
		if err != nil {
			return nil, err
		}
		config.CertificateChain = certificateChain
	}
	return config, nil
}
