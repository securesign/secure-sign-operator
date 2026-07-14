package actions

import (
	"context"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFIPSValidationAction() action.Action[*rhtasv1.Trillian] {
	return fipsAction.NewAction(
		fipsutil.FIPSCondition,
		"trillian",
		fipsAction.Wrapper(fipsAction.Config[*rhtasv1.Trillian]{
			CryptoMaterial: trillianCryptoMaterial,
		}),
	)
}

func trillianCryptoMaterial(ctx context.Context, i *rhtasv1.Trillian, c client.Client) ([]fipsAction.CryptoRef, error) {
	var refs []fipsAction.CryptoRef

	tlsSources := []struct {
		prefix string
		tls    rhtasv1.TLS
	}{
		{"spec.logServer.tls", i.Spec.LogServer.TLS},
		{"spec.logSigner.tls", i.Spec.LogSigner.TLS},
		{"spec.db.tls", i.Spec.Db.TLS},
	}

	for _, src := range tlsSources {
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, src.tls.CertRef,
			src.prefix+".certificateRef", fipsutil.ValidateCertificateChainPEM, &refs); err != nil {
			return nil, err
		}
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, src.tls.PrivateKeyRef,
			src.prefix+".privateKeyRef", fipsutil.ValidatePrivateKeyPEM, &refs); err != nil {
			return nil, err
		}
	}

	return refs, nil
}
