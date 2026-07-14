package server

import (
	"context"
	"encoding/base64"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/utils"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFIPSValidationAction() action.Action[*rhtasv1.Rekor] {
	return fipsAction.NewAction(
		fipsutil.FIPSCondition,
		actions.ServerComponentName,
		fipsAction.Wrapper(fipsAction.Config[*rhtasv1.Rekor]{
			PasswordRef: func(i *rhtasv1.Rekor) *rhtasv1.SecretKeySelector {
				if (i.Spec.Signer.KMS == signerKMSSecret || i.Spec.Signer.KMS == "") && i.Spec.Signer.KeyRef != nil {
					return i.Spec.Signer.PasswordRef //nolint:staticcheck
				}
				return nil
			},
			CryptoMaterial: rekorCryptoMaterial,
		}),
	)
}

func rekorCryptoMaterial(ctx context.Context, i *rhtasv1.Rekor, c client.Client) ([]fipsAction.CryptoRef, error) {
	var refs []fipsAction.CryptoRef

	// Signer key (only for local secret-backed signers, not external KMS)
	if (i.Spec.Signer.KMS == signerKMSSecret || i.Spec.Signer.KMS == "") && i.Spec.Signer.KeyRef != nil {
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.Signer.KeyRef,
			"spec.signer.keyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
			return nil, err
		}
	}

	// Redis TLS material
	if utils.OptionalBool(i.Spec.SearchIndex.Create) {
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.SearchIndex.TLS.CertRef,
			"spec.searchIndex.tls.certificateRef", fipsutil.ValidateCertificateChainPEM, &refs); err != nil {
			return nil, err
		}
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.SearchIndex.TLS.PrivateKeyRef,
			"spec.searchIndex.tls.privateKeyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
			return nil, err
		}
	}

	// Sharding public keys
	for idx, shard := range i.Spec.Sharding {
		if shard.EncodedPublicKey == "" {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(shard.EncodedPublicKey)
		if err != nil {
			return nil, fipsutil.NewValidationError(fmt.Errorf("invalid base64 in spec.sharding[%d].encodedPublicKey: %w", idx, err))
		}
		refs = append(refs, fipsAction.CryptoRef{
			FieldPath: fmt.Sprintf("spec.sharding[%d].encodedPublicKey", idx),
			Data:      decoded,
			Validate:  fipsutil.ValidatePublicKeyPEMOrDER,
		})
	}

	return refs, nil
}
