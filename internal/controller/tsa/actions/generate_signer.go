package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/common"
	"github.com/securesign/operator/internal/controller/common/action"
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
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
	switch {
	case c == nil:
		return false
	case !(c.Reason == constants.Pending || c.Reason == constants.Ready):
		return false
	case !meta.IsStatusConditionTrue(instance.GetConditions(), TSASignerCondition):
		return true
	default:
		return !equality.Semantic.DeepDerivative(instance.Spec.Signer, *instance.Status.Signer)
	}
}

func (g generateSigner) Handle(ctx context.Context, instance *v1alpha1.TimestampAuthority) *action.Result {
	var (
		err error
	)

	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Pending,
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSASignerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Creating,
			ObservedGeneration: instance.Generation,
		})
		return g.StatusUpdate(ctx, instance)
	}

	anno, err := g.secretAnnotations(instance.Spec.Signer)
	if err != nil {
		return g.Failed(err)
	}

	if instance.Status.Signer != nil {
		secret, err := k8sutils.GetSecret(g.Client, instance.Namespace, instance.Status.Signer.CertificateChain.CertificateChainRef.Name)
		if err != nil {
			return g.Failed(fmt.Errorf("can't load CA secret %w", err))
		}

		if equality.Semantic.DeepDerivative(anno, secret.GetAnnotations()) {
			if !meta.IsStatusConditionTrue(instance.GetConditions(), TSASignerCondition) {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:               TSASignerCondition,
					Status:             metav1.ConditionTrue,
					Reason:             "Resolved",
					ObservedGeneration: instance.Generation,
				})
				return g.StatusUpdate(ctx, instance)
			}
			return g.Continue()
		}
	}
	// invalidate
	instance.Status.Signer = instance.Spec.Signer.DeepCopy()

	//Check if a secret for the TSA cert already exists and validate
	partialSecrets, err := k8sutils.ListSecrets(ctx, g.Client, instance.Namespace, TSACertCALabel)
	if err != nil {
		g.Logger.Error(err, "problem with listing secrets", "namespace", instance.Namespace)
	}

	for _, partialSecret := range partialSecrets.Items {
		if equality.Semantic.DeepDerivative(anno, partialSecret.GetAnnotations()) && !meta.IsStatusConditionTrue(instance.GetConditions(), TSASignerCondition) {
			g.alignStatusFields(partialSecret.Name, instance)
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               TSASignerCondition,
				Status:             metav1.ConditionTrue,
				Reason:             "Resolved",
				ObservedGeneration: instance.Generation,
			})
			continue
		}

		// invalidate certificate
		if err := constants.RemoveLabel(ctx, &partialSecret, g.Client, TSACertCALabel); err != nil {
			g.Logger.Error(err, "can't remove label from TSA signer secret", "Name", partialSecret.Name)
		}
		message := fmt.Sprintf("Removed '%s' label from %s secret", TSACertCALabel, partialSecret.Name)
		g.Recorder.Event(instance, v1.EventTypeNormal, "CertificateSecretLabelRemoved", message)
		g.Logger.Info(message)
	}
	if meta.IsStatusConditionTrue(instance.GetConditions(), TSASignerCondition) {
		return g.StatusUpdate(ctx, instance)
	}

	tsaCertChainConfig := &tsaUtils.TsaCertChainConfig{}
	if tsaUtils.IsFileType(instance) {
		tsaCertChainConfig, err = g.handleSignerKeys(instance, tsaCertChainConfig)
		if err != nil {
			g.Logger.Error(err, "error resolving keys for timestamp authority")
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               TSASignerCondition,
				Status:             metav1.ConditionFalse,
				Reason:             constants.Failure,
				Message:            err.Error(),
				ObservedGeneration: instance.Generation,
			})
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:               constants.Ready,
				Status:             metav1.ConditionFalse,
				Reason:             constants.Pending,
				Message:            "Resolving keys",
				ObservedGeneration: instance.Generation,
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
			Type:               TSASignerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Pending,
			Message:            "Resolving keys",
			ObservedGeneration: instance.Generation,
		})
		g.StatusUpdate(ctx, instance)
		// swallow error and retry
		return g.Requeue()
	}

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
	labels[TSACertCALabel] = "certificateChain"

	certificateChain := k8sutils.CreateImmutableSecret(fmt.Sprintf("tsa-signer-config-%s", instance.Name), instance.Namespace, tsaCertChainConfig.ToMap(), labels)
	certificateChain.Annotations = anno
	if _, err := g.Ensure(ctx, certificateChain); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               TSASignerCondition,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:               constants.Ready,
			Status:             metav1.ConditionFalse,
			Reason:             constants.Failure,
			Message:            err.Error(),
			ObservedGeneration: instance.Generation,
		})
		return g.FailedWithStatusUpdate(ctx, err, instance)
	}
	g.Recorder.Event(instance, v1.EventTypeNormal, "TSACertUpdated", "TSA certificate secret updated")
	g.alignStatusFields(certificateChain.Name, instance)
	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               TSASignerCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "Resolved",
		ObservedGeneration: instance.Generation,
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
		if instance.Spec.Signer.CertificateChain.RootCA != nil {
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

		if instance.Spec.Signer.CertificateChain.LeafCA != nil {
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
		if instance.Spec.Signer.CertificateChain.RootCA != nil && instance.Spec.Signer.CertificateChain.LeafCA != nil {
			certificateChain, err := tsaUtils.CreateTSACertChain(ctx, instance, DeploymentName, g.Client, config)
			if err != nil {
				return nil, err
			}
			config.CertificateChain = certificateChain
		}
	}
	return config, nil
}

func (g generateSigner) alignStatusFields(secretName string, instance *v1alpha1.TimestampAuthority) {
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
				Name: secretName,
			},
		}

		if instance.Spec.Signer.File.PrivateKeyRef == nil {
			instance.Status.Signer.File.PrivateKeyRef = &v1alpha1.SecretKeySelector{
				Key: "leafPrivateKey",
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: secretName,
				},
			}
		}

		if instance.Spec.Signer.File.PasswordRef == nil {
			instance.Status.Signer.File.PasswordRef = &v1alpha1.SecretKeySelector{
				Key: "leafPrivateKeyPassword",
				LocalObjectReference: v1alpha1.LocalObjectReference{
					Name: secretName,
				},
			}
		}
	}
}

func (g generateSigner) secretAnnotations(signerConfig v1alpha1.TimestampAuthoritySigner) (map[string]string, error) {
	annotations := make(map[string]string)
	bytes, err := json.Marshal(signerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signer configuration: %w", err)
	}
	annotations[constants.LabelNamespace+"/signerConfiguration"] = string(bytes)
	return annotations, nil
}
