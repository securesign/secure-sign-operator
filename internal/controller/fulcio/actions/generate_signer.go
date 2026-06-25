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
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/fulcio/utils"
	"github.com/securesign/operator/internal/labels"
	"github.com/securesign/operator/internal/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	certSecretNameFormat = "fulcio-cert-config-%s"
)

func NewGenerateSignerAction() action.Action[*rhtasv1.Fulcio] {
	return generateSigner.NewAction(
		CertCondition,
		certSecretNameFormat,
		ComponentName,
		DeploymentName,
		generateSigner.Wrapper(generateSigner.Config[*rhtasv1.Fulcio]{
			Resolve:      resolve,
			GenerateData: generateData,
			AlignStatus:  alignStatus,
			MutateSecret: func(_ *rhtasv1.Fulcio, secret *corev1.Secret) {
				if secret.Labels == nil {
					secret.Labels = make(map[string]string)
				}
				secret.Labels[FulcioCALabel] = constants.KeyCert
			},
		}),
	)
}

func resolve(ctx context.Context, instance *rhtasv1.Fulcio, c client.Client) bool {
	if instance.Spec.Certificate.PrivateKeyRef != nil && instance.Spec.Certificate.CARef != nil {
		commonName, _ := resolveCommonName(ctx, instance, c)
		instance.Status.Certificate = &rhtasv1.FulcioCertStatus{
			PrivateKeyRef:         instance.Spec.Certificate.PrivateKeyRef.DeepCopy(),
			PrivateKeyPasswordRef: instance.Spec.Certificate.PrivateKeyPasswordRef.DeepCopy(), //nolint:staticcheck
			CARef:                 instance.Spec.Certificate.CARef.DeepCopy(),
			CommonName:            commonName,
		}
		return true
	}
	// Upgrade from <1.5.0: check if status references an old GenerateName-based secret
	if instance.Status.Certificate != nil && instance.Status.Certificate.CARef != nil {
		name := instance.Status.Certificate.CARef.Name
		if name != "" && name != fmt.Sprintf(certSecretNameFormat, instance.Name) {
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

func generateData(ctx context.Context, instance *rhtasv1.Fulcio, c client.Client) (map[string][]byte, error) {
	// Validate: CARef without PrivateKeyRef is invalid.
	if instance.Spec.Certificate.PrivateKeyRef == nil && instance.Spec.Certificate.CARef != nil {
		return nil, reconcile.TerminalError(fmt.Errorf("missing private key for CA certificate"))
	}

	commonName, err := resolveCommonName(ctx, instance, c)
	if err != nil {
		return nil, err
	}

	// Cache the resolved commonName in status.
	if instance.Status.Certificate == nil {
		instance.Status.Certificate = &rhtasv1.FulcioCertStatus{}
	}
	instance.Status.Certificate.CommonName = commonName

	config := &utils.FulcioCertConfig{
		OrganizationEmail: instance.Spec.Certificate.OrganizationEmail,
		OrganizationName:  instance.Spec.Certificate.OrganizationName,
		CommonName:        commonName,
	}

	if ref := instance.Spec.Certificate.PrivateKeyPasswordRef; ref != nil {
		password, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKeyPassword = password
	}
	if ref := instance.Spec.Certificate.PrivateKeyRef; ref != nil {
		key, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = key
	} else {
		key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, err
		}

		pemKey, err := utils.CreateCAKey(key)
		if err != nil {
			return nil, err
		}
		config.PrivateKey = pemKey

		pemPubKey, err := utils.CreateCAPub(key.Public())
		if err != nil {
			return nil, err
		}
		config.PublicKey = pemPubKey
	}

	if ref := instance.Spec.Certificate.CARef; ref != nil {
		cert, err := kubernetes.GetSecretData(c, instance.Namespace, ref)
		if err != nil {
			return nil, err
		}
		config.RootCert = cert
	} else {
		rootCert, err := utils.CreateFulcioCA(config)
		if err != nil {
			return nil, err
		}
		config.RootCert = rootCert
	}

	return config.ToData(), nil
}

func alignStatus(instance *rhtasv1.Fulcio, secret *corev1.Secret) {
	if instance.Status.Certificate == nil {
		instance.Status.Certificate = &rhtasv1.FulcioCertStatus{
			PrivateKeyRef:         instance.Spec.Certificate.PrivateKeyRef.DeepCopy(),
			PrivateKeyPasswordRef: instance.Spec.Certificate.PrivateKeyPasswordRef.DeepCopy(), //nolint:staticcheck
			CARef:                 instance.Spec.Certificate.CARef.DeepCopy(),
		}
	}
	if instance.Status.Certificate.CommonName == "" {
		if cn, ok := secret.Annotations[labels.LabelNamespace+"/commonName"]; ok {
			instance.Status.Certificate.CommonName = cn
		}
	}
	if instance.Status.Certificate.PrivateKeyRef == nil {
		instance.Status.Certificate.PrivateKeyRef = &rhtasv1.SecretKeySelector{
			Key: constants.KeyPrivate,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}

	if instance.Spec.Certificate.PrivateKeyPasswordRef != nil {
		// User-provided password reference: copy to status as-is.
		instance.Status.Certificate.PrivateKeyPasswordRef = instance.Spec.Certificate.PrivateKeyPasswordRef.DeepCopy() //nolint:staticcheck
	} else if val, ok := secret.Data[constants.KeyPassword]; ok && len(val) > 0 {
		// No user-provided password, but generated secret includes one.
		instance.Status.Certificate.PrivateKeyPasswordRef = &rhtasv1.SecretKeySelector{
			Key: constants.KeyPassword,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}

	if instance.Spec.Certificate.CARef == nil {
		instance.Status.Certificate.CARef = &rhtasv1.SecretKeySelector{
			Key: constants.KeyCert,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		}
	}
}

func resolveCommonName(ctx context.Context, instance *rhtasv1.Fulcio, c client.Client) (string, error) {
	if instance.Spec.Certificate.CommonName != "" {
		return instance.Spec.Certificate.CommonName, nil
	}
	if instance.Status.Certificate != nil && instance.Status.Certificate.CommonName != "" {
		return instance.Status.Certificate.CommonName, nil
	}
	if !instance.Spec.ExternalAccess.Enabled {
		return fmt.Sprintf("%s.%s.svc.local", DeploymentName, instance.Namespace), nil
	}
	if instance.Spec.ExternalAccess.Host != "" {
		return instance.Spec.ExternalAccess.Host, nil
	}
	return kubernetes.CalculateHostname(ctx, c, DeploymentName, instance.Namespace)
}
