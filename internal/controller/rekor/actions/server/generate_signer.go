package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	generateSigner "github.com/securesign/operator/internal/action/generateSigner"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	signerSecretNameFormat = "rekor-signer-config-%s"
	signerKMSSecret        = "secret"
)

func NewGenerateSignerAction() action.Action[*rhtasv1.Rekor] {
	return generateSigner.NewAction(
		actions.SignerCondition,
		signerSecretNameFormat,
		actions.ServerComponentName,
		actions.ServerDeploymentName,
		generateSigner.Wrapper(generateSigner.Config[*rhtasv1.Rekor]{
			Resolve:      resolve,
			GenerateData: generateData,
			AlignStatus:  alignStatus,
			IsEnabled:    isEnabled,
		}),
	)
}

func isEnabled(instance *rhtasv1.Rekor) bool {
	return instance.Spec.Signer.KMS == signerKMSSecret || instance.Spec.Signer.KMS == ""
}

func resolve(ctx context.Context, instance *rhtasv1.Rekor, c client.Client) bool {
	if instance.Spec.Signer.KeyRef != nil {
		instance.Status.Signer = rhtasv1.RekorSignerStatus{
			KeyRef:      instance.Spec.Signer.KeyRef.DeepCopy(),
			PasswordRef: instance.Spec.Signer.PasswordRef.DeepCopy(), //nolint:staticcheck
		}
		instance.Status.PublicKeyRef = nil
		return true
	}
	// Upgrade from <1.5.0: check if status references an old GenerateName-based secret
	if instance.Status.Signer.KeyRef != nil {
		name := instance.Status.Signer.KeyRef.Name
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

func generateData(_ context.Context, _ *rhtasv1.Rekor, _ client.Client) (map[string][]byte, error) {
	privateKey, publicKey, err := createSignerKey()
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		constants.KeyPrivate: privateKey,
		constants.KeyPublic:  publicKey,
	}, nil
}

func alignStatus(instance *rhtasv1.Rekor, secret *corev1.Secret) {
	instance.Status.Signer = rhtasv1.RekorSignerStatus{
		KeyRef: &rhtasv1.SecretKeySelector{
			Key: constants.KeyPrivate,
			LocalObjectReference: rhtasv1.LocalObjectReference{
				Name: secret.Name,
			},
		},
	}
	instance.Status.PublicKeyRef = nil
}

func createSignerKey() ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	mKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}

	mPubKey, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, nil, err
	}

	var pemPrivKey bytes.Buffer
	if err = pem.Encode(&pemPrivKey, &pem.Block{Type: "EC PRIVATE KEY", Bytes: mKey}); err != nil {
		return nil, nil, err
	}

	var pemPubKey bytes.Buffer
	if err = pem.Encode(&pemPubKey, &pem.Block{Type: "PUBLIC KEY", Bytes: mPubKey}); err != nil {
		return nil, nil, err
	}

	return pemPrivKey.Bytes(), pemPubKey.Bytes(), nil
}
