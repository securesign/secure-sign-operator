package actions

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFIPSValidationAction() action.Action[*rhtasv1.Fulcio] {
	return fipsAction.NewAction(
		fipsutil.FIPSCondition,
		ComponentName,
		fipsAction.Wrapper(fipsAction.Config[*rhtasv1.Fulcio]{
			PasswordRef: func(i *rhtasv1.Fulcio) *rhtasv1.SecretKeySelector {
				if i.Spec.Certificate.PrivateKeyRef != nil {
					return i.Spec.Certificate.PrivateKeyPasswordRef //nolint:staticcheck
				}
				return nil
			},
			CryptoMaterial: func(ctx context.Context, i *rhtasv1.Fulcio, c client.Client) ([]fipsAction.CryptoRef, error) {
				var refs []fipsAction.CryptoRef
				if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.Certificate.PrivateKeyRef,
					"spec.certificate.privateKeyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
					return nil, err
				}
				if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, i.Spec.Certificate.CARef,
					"spec.certificate.caRef", fipsutil.ValidateCertificateChainPEM, &refs); err != nil {
					return nil, err
				}
				return refs, nil
			},
		}),
	)
}
