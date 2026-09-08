package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
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

	for logIdx, log := range i.Spec.Logs {
		if log.Signer != nil && log.Signer.File != nil {
			if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, log.Signer.File.PrivateKeyRef,
				fmt.Sprintf("spec.logs[%d].signer.file.privateKeyRef", logIdx), fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
				return nil, err
			}
			if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, log.Signer.File.PublicKeyRef,
				fmt.Sprintf("spec.logs[%d].signer.file.publicKeyRef", logIdx), fipsutil.ValidatePublicKeyPEM, &refs); err != nil {
				return nil, err
			}
		}
		if log.Signer != nil && log.Signer.PKCS11 != nil {
			if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, log.Signer.PKCS11.PublicKeyRef,
				fmt.Sprintf("spec.logs[%d].signer.pkcs11.publicKeyRef", logIdx), fipsutil.ValidatePublicKeyPEM, &refs); err != nil {
				return nil, err
			}
		}
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

	// Root certificates (from each log in Logs array)
	for logIdx, log := range i.Spec.Logs {
		if len(log.RootCerts) == 0 {
			continue
		}
		for certIdx := range log.RootCerts {
			if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, &log.RootCerts[certIdx],
				fmt.Sprintf("spec.logs[%d].rootCerts[%d]", logIdx, certIdx), fipsutil.ValidateCertificateChainPEM, &refs); err != nil {
				return nil, err
			}
		}
	}

	return refs, nil
}
