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

	// Signer keys from active logs (non-readonly)
	for logIdx, log := range i.Spec.Logs {
		if log.Readonly != nil && *log.Readonly {
			continue // Skip read-only logs
		}
		var privateKeyRef, publicKeyRef *rhtasv1.SecretKeySelector
		if log.Signer != nil && log.Signer.File != nil {
			privateKeyRef = log.Signer.File.PrivateKeyRef
			publicKeyRef = log.Signer.File.PublicKeyRef
		}
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, privateKeyRef,
			fmt.Sprintf("spec.logs[%d].signer.file.privateKeyRef", logIdx), fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
			return nil, err
		}
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, publicKeyRef,
			fmt.Sprintf("spec.logs[%d].signer.file.publicKeyRef", logIdx), fipsutil.ValidatePublicKeyPEM, &refs); err != nil {
			return nil, err
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
