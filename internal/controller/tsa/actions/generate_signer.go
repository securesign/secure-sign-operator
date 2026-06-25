package actions

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	generateSigner "github.com/securesign/operator/internal/action/generateSigner"
	tsaUtils "github.com/securesign/operator/internal/controller/tsa/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	signerSecretNameFormat = "tsa-signer-config-%s"
)

func NewGenerateSignerAction() action.Action[*rhtasv1.TimestampAuthority] {
	return generateSigner.NewAction(
		TSASignerCondition,
		signerSecretNameFormat,
		ComponentName,
		DeploymentName,
		generateSigner.Wrapper(generateSigner.Config[*rhtasv1.TimestampAuthority]{
			Resolve:      resolve,
			GenerateData: generateData,
			AlignStatus:  alignStatusFields,
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

func resolve(ctx context.Context, instance *rhtasv1.TimestampAuthority, c client.Client) bool {
	if instance.Spec.Signer.CertificateChain.CertificateChainRef != nil &&
		instance.Spec.Signer.File != nil &&
		instance.Spec.Signer.File.PrivateKeyRef != nil {
		instance.Status.Signer = signerStatusFromSpec(&instance.Spec.Signer)
		return true
	}
	// Upgrade from <1.5.0: check if status references an old GenerateName-based secret
	if instance.Status.Signer != nil && instance.Status.Signer.CertificateChainRef != nil {
		name := instance.Status.Signer.CertificateChainRef.Name
		if name != "" && name != fmt.Sprintf(signerSecretNameFormat, instance.Name) {
			existing := &corev1.Secret{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: instance.Namespace},
			}), existing); err == nil {
				return true
			}
		}
	}
	return false
}

func generateData(ctx context.Context, instance *rhtasv1.TimestampAuthority, c client.Client) (map[string][]byte, error) {
	tsaCertChainConfig := &tsaUtils.TsaCertChainConfig{}
	var err error

	tsaCertChainConfig, err = handleSignerKeys(instance, tsaCertChainConfig, c)
	if err != nil {
		return nil, err
	}

	tsaCertChainConfig, err = handleCertificateChain(ctx, instance, tsaCertChainConfig, c)
	if err != nil {
		return nil, err
	}

	return tsaCertChainConfig.ToMap(), nil
}

func alignStatusFields(instance *rhtasv1.TimestampAuthority, secret *corev1.Secret) {
	instance.Status.Signer = signerStatusFromSpec(&instance.Spec.Signer)

	if instance.Spec.Signer.File == nil && instance.Spec.Signer.CertificateChain.CertificateChainRef == nil {
		instance.Status.Signer.FileSigner = new(rhtasv1.FileSignerStatus)
	}

	if instance.Status.Signer.CertificateChainRef == nil {
		instance.Status.Signer.CertificateChainRef = &rhtasv1.SecretKeySelector{
			Key: tsaUtils.KeyCertificateChain,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}

	if instance.Status.Signer.FileSigner != nil && instance.Status.Signer.FileSigner.PrivateKeyRef == nil {
		instance.Status.Signer.FileSigner.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key: tsaUtils.KeyLeafPrivateKey,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
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

func handleSignerKeys(instance *rhtasv1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig, c client.Client) (*tsaUtils.TsaCertChainConfig, error) {
	if instance.Spec.Signer.File != nil {
		if instance.Spec.Signer.File.PrivateKeyRef != nil {
			key, err := kubernetes.GetSecretData(c, instance.Namespace, instance.Spec.Signer.File.PrivateKeyRef)
			if err != nil {
				return nil, err
			}
			config.LeafPrivateKey = key

			if ref := instance.Spec.Signer.File.PasswordRef; ref != nil { //nolint:staticcheck
				password, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKeyPassword = password
			}
		}

		if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref == nil {
			key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			if err != nil {
				return nil, err
			}
			rootCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
			if err != nil {
				return nil, err
			}
			config.RootPrivateKey = rootCAPrivKey
		}

	} else {
		if instance.Spec.Signer.CertificateChain.RootCA != nil {
			if ref := instance.Spec.Signer.CertificateChain.RootCA.PrivateKeyRef; ref != nil {
				key, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.RootPrivateKey = key

				if ref := instance.Spec.Signer.CertificateChain.RootCA.PasswordRef; ref != nil { //nolint:staticcheck
					password, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.RootPrivateKeyPassword = password
				}
			} else {
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				rootCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
				if err != nil {
					return nil, err
				}
				config.RootPrivateKey = rootCAPrivKey
			}
		}

		for _, intermediateCA := range instance.Spec.Signer.CertificateChain.IntermediateCA {
			if ref := intermediateCA.PrivateKeyRef; ref != nil {
				key, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, key)

				if ref := intermediateCA.PasswordRef; ref != nil { //nolint:staticcheck
					password, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, password)
				} else {
					config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, []byte(""))
				}
			} else {
				config.IntermediatePrivateKeyPasswords = append(config.IntermediatePrivateKeyPasswords, nil)
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				interCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
				if err != nil {
					return nil, err
				}
				config.IntermediatePrivateKeys = append(config.IntermediatePrivateKeys, interCAPrivKey)
			}
		}

		if instance.Spec.Signer.CertificateChain.LeafCA != nil {
			if ref := instance.Spec.Signer.CertificateChain.LeafCA.PrivateKeyRef; ref != nil {
				key, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKey = key

				if ref := instance.Spec.Signer.CertificateChain.LeafCA.PasswordRef; ref != nil { //nolint:staticcheck
					password, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
					if err != nil {
						return nil, err
					}
					config.LeafPrivateKeyPassword = password
				}
			} else {
				key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				if err != nil {
					return nil, err
				}
				leafCAPrivKey, err := tsaUtils.CreatePrivateKey(key)
				if err != nil {
					return nil, err
				}
				config.LeafPrivateKey = leafCAPrivKey
			}
		}
	}
	return config, nil
}

func handleCertificateChain(ctx context.Context, instance *rhtasv1.TimestampAuthority, config *tsaUtils.TsaCertChainConfig, c client.Client) (*tsaUtils.TsaCertChainConfig, error) {
	if ref := instance.Spec.Signer.CertificateChain.CertificateChainRef; ref != nil {
		certificateChain, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
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
