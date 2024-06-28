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
	TSACertCALabel = constants.LabelNamespace + "/certificate_chain.pem"
)

type handleCerts struct {
	action.BaseAction
}

func NewHandleCertsAction() action.Action[v1alpha1.TimestampAuthority] {
	return &handleCerts{}
}

func (g handleCerts) Name() string {
	return "handle certificate chain"
}

func (g handleCerts) CanHandle(_ context.Context, instance *v1alpha1.TimestampAuthority) bool {
	c := meta.FindStatusCondition(instance.Status.Conditions, constants.Ready)
	return (c.Reason == constants.Pending || c.Reason == constants.Ready) && (instance.Status.Signer == nil ||
		!equality.Semantic.DeepDerivative(instance.Spec.Signer, *instance.Status.Signer))
}

func (g handleCerts) Handle(ctx context.Context, instance *v1alpha1.TimestampAuthority) *action.Result {
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
	secretLabels := map[string]string{
		TSACertCALabel: "cert_chain",
	}
	maps.Copy(secretLabels, labels)

	tsaCertChainConfig, err := g.setupCertificateChain(ctx, instance)
	if err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CertChainCondition,
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

	certificateChain := k8sutils.CreateImmutableSecret(fmt.Sprintf("signer-key-config-%s", instance.Name), instance.Namespace, tsaCertChainConfig.ToMap(), labels)
	if err = controllerutil.SetControllerReference(instance, certificateChain, g.Client.Scheme()); err != nil {
		return g.Failed(fmt.Errorf("could not set controller reference for Secret: %w", err))
	}
	if err = g.Client.DeleteAllOf(ctx, &v1.Secret{}, client.InNamespace(instance.Namespace), client.MatchingLabels(constants.LabelsFor(ComponentName, DeploymentName, instance.Name)), client.HasLabels{TSACertCALabel}); err != nil {
		return g.Failed(err)
	}
	if _, err := g.Ensure(ctx, certificateChain); err != nil {
		meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
			Type:    CertChainCondition,
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
	instance.Spec.Signer.DeepCopyInto(instance.Status.Signer)

	if instance.Spec.Signer.CertificateChain.CertificateChainRef == nil {
		instance.Status.Signer.CertificateChain.CertificateChainRef = &v1alpha1.SecretKeySelector{
			Key: "certificateChain",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: certificateChain.Name,
			},
		}
	}

	if instance.Spec.Signer.FileSigner.PrivateKeyRef == nil {
		instance.Status.Signer.FileSigner.PrivateKeyRef = &v1alpha1.SecretKeySelector{
			Key: "interPrivateKey",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: certificateChain.Name,
			},
		}
	}

	if instance.Spec.Signer.FileSigner.PasswordRef == nil && len(tsaCertChainConfig.InterPrivateKeyPassword) > 0 {
		instance.Status.Signer.FileSigner.PasswordRef = &v1alpha1.SecretKeySelector{
			Key: "interPrivateKeyPassword",
			LocalObjectReference: v1alpha1.LocalObjectReference{
				Name: certificateChain.Name,
			},
		}
	}

	meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:   CertChainCondition,
		Status: metav1.ConditionTrue,
		Reason: "Resolved",
	})
	return g.StatusUpdate(ctx, instance)
}

func (g handleCerts) setupCertificateChain(ctx context.Context, instance *v1alpha1.TimestampAuthority) (*tsaUtils.TsaCertChainConfig, error) {
	config := &tsaUtils.TsaCertChainConfig{}

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
		if ref := instance.Spec.Signer.CertificateChain.RootPrivateKeyRef; ref != nil {
			key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
			if err != nil {
				return nil, err
			}
			config.RootPrivateKey = key

			if ref := instance.Spec.Signer.CertificateChain.RootPasswordRef; ref != nil {
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

		if ref := instance.Spec.Signer.CertificateChain.InterPrivateKeyRef; ref != nil {
			key, err := k8sutils.GetSecretData(g.Client, instance.Namespace, ref)
			if err != nil {
				return nil, err
			}
			config.InterPrivateKey = key

			if ref := instance.Spec.Signer.CertificateChain.InterPasswordRef; ref != nil {
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
