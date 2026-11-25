package trustroot

import (
	"cmp"
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func LoadFulcioIssuers(ctx context.Context, cli client.Client, namespace string, bindings []rhtasv1.TrustRootBindingWithOIDC, instance *rhtasv1.Fulcio) ([]string, error) {
	if len(bindings) == 1 && bindings[0].OIDCIssuers != nil {
		return bindings[0].OIDCIssuers, nil
	}

	if err := ensureFetched(ctx, cli, namespace, FirstBinding(bindings).GetServiceRef(), instance); err != nil {
		return nil, fmt.Errorf("resolve Fulcio OIDC issuers: %w", err)
	}

	var issuers []string
	for _, issuer := range instance.Spec.Config.OIDCIssuers {
		if url := cmp.Or(issuer.IssuerURL, issuer.Issuer); url != "" {
			issuers = append(issuers, url)
		}
	}
	return issuers, nil
}

func FirstBinding[T TrustRootBinder](bindings []T) TrustRootBinder {
	if len(bindings) == 0 {
		return rhtasv1.TrustRootBinding{}
	}
	return bindings[0]
}
