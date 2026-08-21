package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	ctlogUtils "github.com/securesign/operator/internal/controller/ctlog/utils"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFIPSValidationAction() action.Action[*rhtasv1.CTlog] {
	return fipsAction.NewAction(
		fipsutil.FIPSCondition,
		ComponentName,
		fipsAction.Wrapper(fipsAction.Config[*rhtasv1.CTlog]{
			CryptoMaterial: ctlogCryptoMaterial,
		}),
	)
}

func ctlogCryptoMaterial(ctx context.Context, i *rhtasv1.CTlog, c client.Client) ([]fipsAction.CryptoRef, error) {
	var refs []fipsAction.CryptoRef

	// Signer keys
	var privateKeyRef, publicKeyRef *rhtasv1.SecretKeySelector
	if i.Spec.Signer.File != nil {
		privateKeyRef = i.Spec.Signer.File.PrivateKeyRef
		publicKeyRef = i.Spec.Signer.File.PublicKeyRef
	}
	if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, privateKeyRef,
		"spec.signer.file.privateKeyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
		return nil, err
	}
	if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, publicKeyRef,
		"spec.signer.file.publicKeyRef", fipsutil.ValidatePublicKeyPEM, &refs); err != nil {
		return nil, err
	}

	// TLS material
	if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.TLS.CertRef,
		"spec.tls.certificateRef", fipsutil.ValidateCertificateChainPEM, &refs); err != nil {
		return nil, err
	}
	if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.TLS.PrivateKeyRef,
		"spec.tls.privateKeyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
		return nil, err
	}

	// Root certificates (populated by HandleFulcioCertAction before this action runs)
	for idx := range i.Status.RootCertificates {
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, &i.Status.RootCertificates[idx],
			fmt.Sprintf("status.rootCertificates[%d]", idx), fipsutil.ValidateCertificateChainPEM, &refs); err != nil {
			return nil, err
		}
	}

	// Shard keys (for inactive shards in sharding configuration)
	for idx, shard := range i.Spec.Sharding {
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, &shard.PublicKeyRef,
			fmt.Sprintf("spec.sharding[%d].publicKeyRef", idx), fipsutil.ValidatePublicKeyPEM, &refs); err != nil {
			return nil, err
		}
		if shard.PrivateKeyRef != nil {
			if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, shard.PrivateKeyRef,
				fmt.Sprintf("spec.sharding[%d].privateKeyRef", idx), fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
				return nil, err
			}
		}
	}

	// Custom server config crypto material
	if i.Spec.ServerConfigRef != nil {
		secret, err := kubernetes.GetSecret(ctx, c, i.Namespace, i.Spec.ServerConfigRef.Name)
		if err != nil {
			return nil, err
		}
		for _, key := range slices.Sorted(maps.Keys(secret.Data)) {
			if key == ctlogUtils.ConfigKey || key == ctlogUtils.Password {
				continue
			}
			refs = append(refs, fipsAction.CryptoRef{
				FieldPath: fmt.Sprintf("spec.serverConfigRef[%s]", key),
				Data:      secret.Data[key],
				Validate:  fipsutil.ValidateCryptoMaterialIfPEM,
			})
		}
	}

	return refs, nil
}
