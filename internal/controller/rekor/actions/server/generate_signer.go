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
			ResolveRef:   resolveRef,
			GenerateData: generateData,
			AlignStatus:  alignStatus,
			IsEnabled:    isEnabled,
		}),
	)
}

func isEnabled(instance *rhtasv1.Rekor) bool {
	return instance.Spec.Signer.KMS == signerKMSSecret || instance.Spec.Signer.KMS == ""
}

func resolveRef(ctx context.Context, instance *rhtasv1.Rekor, c client.Client) (*rhtasv1.SecretKeySelector, error) {
	if instance.Spec.Signer.KeyRef != nil {
		if err := generateSigner.RequireSecret(ctx, c, instance.Namespace, instance.Spec.Signer.KeyRef); err != nil {
			return nil, err
		}
		return instance.Spec.Signer.KeyRef, nil
	}
	return generateSigner.ResolveStatusSecret(ctx, c, instance.Status.Signer.KeyRef, instance.Namespace, fmt.Sprintf(signerSecretNameFormat, instance.Name))
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

func alignStatus(instance *rhtasv1.Rekor, ref rhtasv1.SecretKeySelector) {
	if instance.Spec.Signer.KeyRef != nil {
		instance.Status.Signer = rhtasv1.RekorSignerStatus{
			KeyRef:      instance.Spec.Signer.KeyRef.DeepCopy(),
			PasswordRef: instance.Spec.Signer.PasswordRef.DeepCopy(), //nolint:staticcheck
		}
	} else {
		instance.Status.Signer = rhtasv1.RekorSignerStatus{
			KeyRef: &rhtasv1.SecretKeySelector{
				Key:                  constants.KeyPrivate,
				LocalObjectReference: ref.LocalObjectReference,
			},
		}
	}
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
