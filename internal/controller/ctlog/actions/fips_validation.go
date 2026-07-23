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
			PasswordRef: func(i *rhtasv1.CTlog) *rhtasv1.SecretKeySelector {
				if i.Spec.PrivateKeyRef != nil {
					return i.Spec.PrivateKeyPasswordRef //nolint:staticcheck
				}
				return nil
			},
			CryptoMaterial: ctlogCryptoMaterial,
		}),
	)
}

func ctlogCryptoMaterial(ctx context.Context, i *rhtasv1.CTlog, c client.Client) ([]fipsAction.CryptoRef, error) {
	var refs []fipsAction.CryptoRef

	// Signer keys
	if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.PrivateKeyRef,
		"spec.privateKeyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
		return nil, err
	}
	if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.PublicKeyRef,
		"spec.publicKeyRef", fipsutil.ValidatePublicKeyPEM, &refs); err != nil {
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
