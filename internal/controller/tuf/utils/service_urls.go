package utils

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var ErrorResolveServiceUrl = fmt.Errorf("failed to resolve service url")

type resolvedServiceAddressResult struct {
	Address     string
	OIDCIssuers []string
}

var keyRefBinding = map[string]struct {
	serviceRef func(instance *rhtasv1.Tuf) apis.ServiceReferencer
	instance   apis.AddressableObject
}{
	rhtasv1.TufKeyRekor: {
		serviceRef: func(instance *rhtasv1.Tuf) apis.ServiceReferencer {
			return instance.Spec.Rekor
		},
		instance: &rhtasv1.Rekor{},
	},
	rhtasv1.TufKeyCTFE: {
		serviceRef: func(instance *rhtasv1.Tuf) apis.ServiceReferencer {
			return instance.Spec.Ctlog
		},
		instance: &rhtasv1.CTlog{},
	},
	rhtasv1.TufKeyFulcio: {
		serviceRef: func(instance *rhtasv1.Tuf) apis.ServiceReferencer {
			return instance.Spec.Fulcio
		},
		instance: &rhtasv1.Fulcio{},
	},
	rhtasv1.TufKeyTSA: {
		serviceRef: func(instance *rhtasv1.Tuf) apis.ServiceReferencer {
			return instance.Spec.Tsa
		},
		instance: &rhtasv1.TimestampAuthority{},
	},
}

func resolveServiceAddress(ctx context.Context, c client.Client, instance *rhtasv1.Tuf, keyName string) (*resolvedServiceAddressResult, error) {
	var oidcIssuers []string
	binding, ok := keyRefBinding[keyName]
	if !ok {
		return nil, fmt.Errorf("unknown key %s", keyName)
	}
	url, err := utils.ResolveExternalServiceUrl(ctx, c, binding.serviceRef(instance), instance.Namespace, binding.instance)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrorResolveServiceUrl, err)
	}

	if withOidc, ok := binding.serviceRef(instance).(rhtasv1.ServiceRefWithOIDC); ok {
		if len(withOidc.OIDCIssuers) > 0 {
			oidcIssuers = append(oidcIssuers, withOidc.OIDCIssuers...)
		} else if fulcioInstance, ok := binding.instance.(*rhtasv1.Fulcio); ok {
			for _, oidc := range fulcioInstance.Spec.Config.OIDCIssuers {
				if oidc.IssuerURL != "" {
					oidcIssuers = append(oidcIssuers, oidc.IssuerURL)
				}
			}
		} else {
			log.FromContext(ctx).Info("service does not support OIDC issuers", "type", fmt.Sprintf("%T", binding.instance))
		}
	}
	return &resolvedServiceAddressResult{
		Address:     url,
		OIDCIssuers: oidcIssuers,
	}, nil
}
