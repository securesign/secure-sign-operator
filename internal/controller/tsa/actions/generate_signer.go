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
	k8sutils "github.com/securesign/operator/internal/controller/common/utils/kubernetes"
	"github.com/securesign/operator/internal/controller/constants"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	TSACertCALabel = constants.LabelNamespace + "/tsa_cert_chain.pem"
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
	if meta.FindStatusCondition(instance.Status.Conditions, constants.Ready).Reason != constants.Pending {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:   constants.Ready,
			Status: metav1.ConditionFalse,
			Reason: constants.Pending,
		},
		)
		return g.StatusUpdate(ctx, instance)
	}

	labels := constants.LabelsFor(ComponentName, DeploymentName, instance.Name)
	tsaCertChainConfig, err := g.setupCertificateChain(ctx, instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    TSAServerCondition,
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
		return g.FailedWithStatusUpdate(ctx, err, instance)
	}

	secretLabels := map[string]string{
		TSACertCALabel: "certificateChain",
	}
	maps.Copy(secretLabels, labels)

	certificateChain := k8sutils.CreateImmutableSecret(fmt.Sprintf("tsa-signer-config-%s", instance.Name), instance.Namespace, tsaCertChainConfig.ToMap(), secretLabels)
	if err = controllerutil.SetControllerReference(instance, certificateChain, g.Client.Scheme()); err != nil {
		return g.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}
	if err = g.Client.DeleteAllOf(ctx, &v1.Secret{}, client.InNamespace(instance.Namespace), client.MatchingLabels(constants.LabelsFor(ComponentName, DeploymentName, instance.Name)), client.HasLabels{TSACertCALabel}); err != nil {
		return g.Failed(err)
	}
	if _, err := g.Ensure(ctx, certificateChain); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    TSAServerCondition,
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
	if instance.Spec.Signer.FileSigner == nil && tsaUtils.IsFileType(instance) {
		instance.Spec.Signer.FileSigner = new(v1alpha1.FileSigner)
	}
	instance.Spec.Signer.DeepCopyInto(instance.Status.Signer)

	if instance.Spec.Signer.CertificateChain.CertificateChainRef == nil {
		instance.Status.Signer.CertificateChain.CertificateChainRef = &v1alpha1.SecretKeySelector{
			Key: "certificateChain",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: certificateChain.Name,
			},
		}
	}

	if instance.Spec.Signer.FileSigner.PrivateKeyRef == nil && tsaUtils.IsFileType(instance) {
		instance.Status.Signer.FileSigner.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			Key: "interPrivateKey",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: certificateChain.Name,
			},
		}
	}

	if instance.Spec.Signer.FileSigner.PasswordRef == nil && len(tsaCertChainConfig.InterPrivateKeyPassword) > 0 && tsaUtils.IsFileType(instance) {
		instance.Status.Signer.FileSigner.PasswordRef = &v1alpha1.SecretKeySelector{
			Key: "interPrivateKeyPassword",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: certificateChain.Name,
			},
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   TSAServerCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	})
	return g.StatusUpdate(ctx, instance)
}

func (g generateSigner) setupCertificateChain(ctx context.Context, instance *v1alpha1.TimestampAuthority) (*tsaUtils.TsaCertChainConfig, error) {
	config := &tsaUtils.TsaCertChainConfig{}

	if tsaUtils.IsFileType(instance) {
		if instance.Spec.Signer.FileSigner != nil {

			if instance.Spec.Signer.FileSigner.PrivateKeyRef != nil {
				key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, instance.Spec.Signer.FileSigner.PrivateKeyRef)
				if err != nil {
					return nil, err
				}
				config.InterPrivateKey = key

				if ref := instance.Spec.Signer.FileSigner.PasswordRef; ref != nil {
					password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.InterPrivateKeyPassword = password
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

			if ref := instance.Spec.Signer.CertificateChain.IntermediateCA.PrivateKeyRef; ref != nil {
				key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.InterPrivateKey = key

				if ref := instance.Spec.Signer.CertificateChain.IntermediateCA.PasswordRef; ref != nil {
					password, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.InterPrivateKeyPassword = password
				}
			} else {
				config.InterPrivateKeyPassword = common.GeneratePassword(8)
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				interCAPrivKey, err := tsaUtils.CreatePrivateKey(key, config.InterPrivateKeyPassword)
				if err != nil {
					return nil, err
				}
				config.InterPrivateKey = interCAPrivKey
			}
		}
	}

	if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref != nil {
		certificateChain, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.CertificateChain = certificateChain
	} else {
		if tsaUtils.IsFileType(instance) {
			certificateChain, err := tsaUtils.CreateTSACertChain(ctx, instance, DeploymentName, g.Client, config)
			if err != nil {
				return nil, err
			}
			config.CertificateChain = certificateChain
		}
	}
	return config, nil
}
