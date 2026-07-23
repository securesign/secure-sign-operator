package actions

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFIPSValidationAction() action.Action[*rhtasv1.TimestampAuthority] {
	return fipsAction.NewAction(
		fipsutil.FIPSCondition,
		ComponentName,
		fipsAction.Wrapper(fipsAction.Config[*rhtasv1.TimestampAuthority]{
			PasswordRef: func(i *rhtasv1.TimestampAuthority) *rhtasv1.SecretKeySelector {
				if i.Spec.Signer.File != nil && i.Spec.Signer.File.PrivateKeyRef != nil {
					return i.Spec.Signer.File.PasswordRef //nolint:staticcheck
				}
				return nil
			},
			CryptoMaterial: func(ctx context.Context, i *rhtasv1.TimestampAuthority, c client.Client) ([]fipsAction.CryptoRef, error) {
				var refs []fipsAction.CryptoRef
				if i.Spec.Signer.File != nil {
					if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.Signer.File.PrivateKeyRef,
						"spec.signer.file.privateKeyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
						return nil, err
					}
				}
				if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.Signer.CertificateChain.CertificateChainRef,
					"spec.signer.certificateChain.certificateChainRef", fipsutil.ValidateCertificateChainPEM, &refs); err != nil {
					return nil, err
				}
				return refs, nil
			},
		}),
	)
}
