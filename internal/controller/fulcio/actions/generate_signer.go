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
	generateSigner "github.com/securesign/operator/internal/action/generateSigner"
	"github.com/securesign/operator/internal/constants"
	fulcioutils "github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	certSecretNameFormat = "fulcio-cert-config-%s"
)

var (
	ErrMissingPrivateKey = errors.New("missing private key for CA certificate")
	ErrMissingCACert     = errors.New("missing CA certificate for private key")
)

func NewGenerateSignerAction() action.Action[*rhtasv1.Fulcio] {
	return generateSigner.NewAction(
		CertCondition,
		certSecretNameFormat,
		ComponentName,
		DeploymentName,
		generateSigner.Wrapper(generateSigner.Config[*rhtasv1.Fulcio]{
			ResolveRef:   resolveRef,
			GenerateData: generateData,
			AlignStatus:  alignStatus,
			IsEnabled: func(i *rhtasv1.Fulcio) bool {
				return i.Spec.Signer.Type == rhtasv1.SignerTypeFile || i.Spec.Signer.Type == ""
			},
			MutateSecret: func(_ *rhtasv1.Fulcio, secret *corev1.Secret) {
				if secret.Labels == nil {
					secret.Labels = make(map[string]string)
				}
				secret.Labels[FulcioCALabel] = constants.KeyCert
			},
		}),
	)
}

func resolveRef(ctx context.Context, instance *rhtasv1.Fulcio, c client.Client) (*rhtasv1.SecretKeySelector, error) {
	var privateKeyRef *rhtasv1.SecretKeySelector
	if instance.Spec.Signer.File != nil {
		privateKeyRef = instance.Spec.Signer.File.PrivateKeyRef
	}
	certificateChainRef := instance.Spec.Signer.CertificateChain.CertificateChainRef
	if privateKeyRef == nil && certificateChainRef != nil {
		return nil, reconcile.TerminalError(ErrMissingPrivateKey)
	}
	if privateKeyRef != nil && certificateChainRef == nil {
		return nil, reconcile.TerminalError(ErrMissingCACert)
	}
	if privateKeyRef != nil && certificateChainRef != nil {
		if err := generateSigner.RequireSecret(ctx, c, instance.Namespace, privateKeyRef); err != nil {
			return nil, err
		}
		if err := generateSigner.RequireSecret(ctx, c, instance.Namespace, certificateChainRef); err != nil {
			return nil, err
		}
		return certificateChainRef, nil
	}
	var ref *rhtasv1.SecretKeySelector
	if instance.Status.Certificate != nil {
		ref = instance.Status.Certificate.CARef
	}
	return generateSigner.ResolveStatusSecret(ctx, c, ref, instance.Namespace, fmt.Sprintf(certSecretNameFormat, instance.Name))
}

func generateData(ctx context.Context, instance *rhtasv1.Fulcio, c client.Client) (map[string][]byte, error) {
	commonName, err := resolveCommonName(ctx, instance, c)
	if err != nil {
		return nil, err
	}

	config := &fulcioutils.FulcioCertConfig{
		OrganizationEmail: instance.Spec.Signer.CertificateChain.OrganizationEmail,
		OrganizationName:  instance.Spec.Signer.CertificateChain.OrganizationName,
		CommonName:        commonName,
	}

	file := instance.Spec.Signer.File
	if file != nil && file.PrivateKeyPasswordRef != nil { //nolint:staticcheck
		password, err := kubernetes.GetSecretData(ctx, c, instance.Namespace, file.PrivateKeyPasswordRef) //nolint:staticcheck
		if err != nil {
			return nil, err
		}
		config.PrivateKeyPassword = password
	}
	if file != nil && file.PrivateKeyRef != nil {
		key, err := kubernetes.GetSecretData(ctx, c, instance.Namespace, file.PrivateKeyRef)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = key
	} else {
		key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, err
		}

		pemKey, err := fulcioutils.CreateCAKey(key)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = pemKey

		pemPubKey, err := fulcioutils.CreateCAPub(key.Public())
		if err != nil {
			return nil, err
		}
		config.PublicKey = pemPubKey
	}

	if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref != nil {
		cert, err := kubernetes.GetSecretData(ctx, c, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.RootCert = cert
	} else {
		rootCert, err := fulcioutils.CreateFulcioCA(config)
		if err != nil {
			return nil, err
		}
		config.RootCert = rootCert
	}

	return config.ToData(), nil
}

func alignStatus(instance *rhtasv1.Fulcio, ref rhtasv1.SecretKeySelector) {
	if instance.Status.Certificate == nil {
		instance.Status.Certificate = &rhtasv1.FulcioCertStatus{}
	}
	file := instance.Spec.Signer.File
	if file != nil && file.PrivateKeyRef != nil {
		instance.Status.Certificate.PrivateKeyRef = file.PrivateKeyRef.DeepCopy()
	} else {
		instance.Status.Certificate.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyPrivate,
			LocalObjectReference: ref.LocalObjectReference,
		}
	}
	if file != nil && file.PrivateKeyPasswordRef != nil { //nolint:staticcheck
		instance.Status.Certificate.PrivateKeyPasswordRef = file.PrivateKeyPasswordRef.DeepCopy() //nolint:staticcheck
	}
	if instance.Spec.Signer.CertificateChain.CertificateChainRef != nil {
		instance.Status.Certificate.CARef = instance.Spec.Signer.CertificateChain.CertificateChainRef.DeepCopy()
	} else {
		instance.Status.Certificate.CARef = &rhtasv1.SecretKeySelector{
			Key:                  constants.KeyCert,
			LocalObjectReference: ref.LocalObjectReference,
		}
	}
}

func resolveCommonName(ctx context.Context, instance *rhtasv1.Fulcio, c client.Client) (string, error) {
	if instance.Spec.Signer.CertificateChain.CommonName != "" {
		return instance.Spec.Signer.CertificateChain.CommonName, nil
	}
	if !utils.IsEnabled(instance.Spec.Ingress.Enabled) {
		return fmt.Sprintf("%s.%s.svc.local", DeploymentName, instance.Namespace), nil
	}
	if instance.Spec.Ingress.Host != "" {
		return instance.Spec.Ingress.Host, nil
	}
	return kubernetes.CalculateHostname(ctx, c, DeploymentName, instance.Namespace)
}
