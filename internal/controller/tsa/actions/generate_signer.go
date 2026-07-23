package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/generateSigner"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	signerSecretNameFormat = "tsa-signer-config-%s"
)

var ErrMissingPrivateKey = errors.New("missing private key for certificate chain")

func NewGenerateSignerAction() action.Action[*rhtasv1.TimestampAuthority] {
	return generateSigner.NewAction(
		TSASignerCondition,
		signerSecretNameFormat,
		ComponentName,
		DeploymentName,
		generateSigner.Wrapper(generateSigner.Config[*rhtasv1.TimestampAuthority]{
			ResolveRef:   resolveRef,
			GenerateData: generateData,
			AlignStatus:  alignStatus,
			IsEnabled:    isEnabled,
			MutateSecret: func(_ *rhtasv1.TimestampAuthority, secret *corev1.Secret) {
				if secret.Labels == nil {
					secret.Labels = make(map[string]string)
				}
				secret.Labels[labels.LabelNamespace+"/tsa.certchain.pem"] = tsaUtils.KeyCertificateChain
			},
		}),
	)
}

func isEnabled(instance *rhtasv1.TimestampAuthority) bool {
	return tsaUtils.IsFileType(instance)
}

func resolveRef(ctx context.Context, instance *rhtasv1.TimestampAuthority, c client.Client) (*rhtasv1.SecretKeySelector, error) {
	if instance.Spec.Signer.CertificateChain.CertificateChainRef != nil &&
		instance.Spec.Signer.File != nil &&
		instance.Spec.Signer.File.PrivateKeyRef == nil {
		return nil, reconcile.TerminalError(ErrMissingPrivateKey)
	}
	if instance.Spec.Signer.CertificateChain.CertificateChainRef != nil &&
		instance.Spec.Signer.File != nil &&
		instance.Spec.Signer.File.PrivateKeyRef != nil {
		if err := generateSigner.RequireSecret(ctx, c, instance.Namespace, instance.Spec.Signer.File.PrivateKeyRef); err != nil {
			return nil, err
		}
		if err := generateSigner.RequireSecret(ctx, c, instance.Namespace, instance.Spec.Signer.CertificateChain.CertificateChainRef); err != nil {
			return nil, err
		}
		return instance.Spec.Signer.CertificateChain.CertificateChainRef, nil
	}
	var ref *rhtasv1.SecretKeySelector
	if instance.Status.Signer != nil {
		ref = instance.Status.Signer.CertificateChainRef
	}
	return generateSigner.ResolveStatusSecret(ctx, c, ref, instance.Namespace, fmt.Sprintf(signerSecretNameFormat, instance.Name))
}

func generateData(ctx context.Context, instance *rhtasv1.TimestampAuthority, c client.Client) (map[string][]byte, error) {
	tsaCertChainConfig := &tsaUtils.TsaCertChainConfig{}
	var err error

	tsaCertChainConfig, err = handleSignerKeys(instance, tsaCertChainConfig)
	if err != nil {
		return nil, err
	}

	tsaCertChainConfig, err = handleCertificateChain(ctx, instance, tsaCertChainConfig, c)
	if err != nil {
		return nil, err
	}

	return tsaCertChainConfig.ToMap(), nil
}

func alignStatus(instance *rhtasv1.TimestampAuthority, ref rhtasv1.SecretKeySelector) {
	instance.Status.Signer = signerStatusFromSpec(&instance.Spec.Signer)

	if instance.Spec.Signer.File == nil && instance.Spec.Signer.CertificateChain.CertificateChainRef == nil {
		instance.Status.Signer.FileSigner = new(rhtasv1.FileSignerStatus)
	}

	if instance.Status.Signer.CertificateChainRef == nil {
		instance.Status.Signer.CertificateChainRef = &rhtasv1.SecretKeySelector{
			Key:                  tsaUtils.KeyCertificateChain,
			LocalObjectReference: ref.LocalObjectReference,
		}
	}

	if instance.Status.Signer.FileSigner != nil && instance.Status.Signer.FileSigner.PrivateKeyRef == nil {
		instance.Status.Signer.FileSigner.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key:                  tsaUtils.KeyLeafPrivateKey,
			LocalObjectReference: ref.LocalObjectReference,
		}
	}
}

func signerStatusFromSpec(signer *rhtasv1.TimestampAuthoritySigner) *rhtasv1.TimestampAuthoritySignerStatus {
	status := &rhtasv1.TimestampAuthoritySignerStatus{
		CertificateChainRef: signer.CertificateChain.CertificateChainRef.DeepCopy(),
	}
	if signer.File != nil {
		status.FileSigner = &rhtasv1.FileSignerStatus{
			PrivateKeyRef: signer.File.PrivateKeyRef.DeepCopy(),
		}
		if signer.File.PasswordRef != nil { //nolint:staticcheck
			status.FileSigner.PasswordRef = signer.File.PasswordRef.DeepCopy() //nolint:staticcheck
		}
	}
	return status
}

func generatePrivateKey() ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return tsaUtils.CreatePrivateKey(key)
}

func handleSignerKeys(instance *rhtasv1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig) (*tsaUtils.TsaCertChainConfig, error) {
	if instance.Spec.Signer.CertificateChain.RootCA != nil {
		rootKey, err := generatePrivateKey()
		if err != nil {
			return nil, err
		}
		config.RootPrivateKey = rootKey
	}

	for range instance.Spec.Signer.CertificateChain.IntermediateCA {
		interKey, err := generatePrivateKey()
		if err != nil {
			return nil, err
		}
		config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, interKey)
		config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, nil)
	}

	if instance.Spec.Signer.CertificateChain.LeafCA != nil {
		leafKey, err := generatePrivateKey()
		if err != nil {
			return nil, err
		}
		config.LeafPrivateKey = leafKey
	}

	return config, nil
}

func handleCertificateChain(ctx context.Context, instance *rhtasv1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig, c client.Client) (*tsaUtils.TsaCertChainConfig, error) {
	if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref != nil {
		certificateChain, err := kubernetes.GetSecretData(ctx, c, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.CertificateChain = certificateChain
	} else {
		if instance.Spec.Signer.CertificateChain.RootCA != nil && instance.Spec.Signer.CertificateChain.LeafCA != nil {
			certificateChain, err := tsaUtils.CreateTSACertChain(ctx, instance, DeploymentName, c, config)
			if err != nil {
				return nil, err
			}
			config.CertificateChain = certificateChain
		}
	}
	return config, nil
}
