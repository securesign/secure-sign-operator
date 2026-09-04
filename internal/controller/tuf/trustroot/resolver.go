// Package trustroot resolves a TUF trust root component's service address and
// trust material through one three-tier flow: explicit secretRef/URL, in-cluster
// ref, or autodiscovery-by-listing. Address and material resolve independently,
// sharing a single cluster fetch of the sibling component instance when both
// need it.
package trustroot

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/serviceresolver"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ComponentKey identifies a trust root component. Values match the well-known
// TUF target filenames (rhtasv1.TufKeyRekor etc.) so they double as
// TufStatus.Keys entries and condition-type names, with no separate lookup table.
type ComponentKey string

const (
	Rekor  ComponentKey = ComponentKey(rhtasv1.TufKeyRekor)
	CTFE   ComponentKey = ComponentKey(rhtasv1.TufKeyCTFE)
	Fulcio ComponentKey = ComponentKey(rhtasv1.TufKeyFulcio)
	TSA    ComponentKey = ComponentKey(rhtasv1.TufKeyTSA)
)

func (k ComponentKey) String() string { return string(k) }

// ActiveKeys lists the components included in instance's trust root: Rekor/CTlog/Fulcio
// always, TSA only when instance.Spec.Tsa is non-nil (nil excludes TSA from the trust
// root entirely).
func ActiveKeys(instance *rhtasv1.Tuf) []ComponentKey {
	keys := []ComponentKey{Rekor, CTFE, Fulcio}
	if instance.Spec.Tsa != nil {
		keys = append(keys, TSA)
	}
	return keys
}

// Binding returns the configured binding for key in instance.Spec, or a zero-value
// binding (which triggers autodiscovery) when the list is empty or key is Fulcio
// with no configured entry. Use FulcioBinding instead for Fulcio's OIDC issuers.
func Binding(instance *rhtasv1.Tuf, key ComponentKey) rhtasv1.TrustRootBinding {
	switch key {
	case Rekor:
		return firstBinding(instance.Spec.Rekor)
	case CTFE:
		return firstBinding(instance.Spec.Ctlog)
	case Fulcio:
		return FulcioBinding(instance).TrustRootBinding
	case TSA:
		if instance.Spec.Tsa == nil {
			return rhtasv1.TrustRootBinding{}
		}
		return firstBinding(*instance.Spec.Tsa)
	default:
		return rhtasv1.TrustRootBinding{}
	}
}

// FulcioBinding returns instance's configured Fulcio binding, or a zero-value
// binding (autodiscovery, no static OIDC issuers) when unconfigured.
func FulcioBinding(instance *rhtasv1.Tuf) rhtasv1.TrustRootBindingWithOIDC {
	if len(instance.Spec.Fulcio) == 0 {
		return rhtasv1.TrustRootBindingWithOIDC{}
	}
	return instance.Spec.Fulcio[0]
}

// firstBinding returns bindings[0], or a zero-value binding when empty which falls
// through to autodiscovery.
func firstBinding(bindings []rhtasv1.TrustRootBinding) rhtasv1.TrustRootBinding {
	if len(bindings) == 0 {
		return rhtasv1.TrustRootBinding{}
	}
	return bindings[0]
}

var (
	ErrUnknownComponent  = errors.New("unknown trust root component")
	ErrResolveAddress    = errors.New("failed to resolve component address")
	ErrResolveMaterial   = errors.New("failed to resolve trust material")
	ErrComponentNotReady = errors.New("component instance not ready")
)

// Resolved is a component's resolved service address and trust material.
type Resolved struct {
	Address     string
	Material    []byte
	OIDCIssuers []string
}

type descriptor struct {
	newInstance        func() apis.AddressableConditionAware
	materialFromStatus func(apis.AddressableConditionAware) string
}

var descriptors = map[ComponentKey]descriptor{
	Rekor: {
		newInstance:        func() apis.AddressableConditionAware { return &rhtasv1.Rekor{} },
		materialFromStatus: func(obj apis.AddressableConditionAware) string { return obj.(*rhtasv1.Rekor).Status.PublicKey },
	},
	CTFE: {
		newInstance: func() apis.AddressableConditionAware { return &rhtasv1.CTlog{} },
		materialFromStatus: func(obj apis.AddressableConditionAware) string {
			ctlog := obj.(*rhtasv1.CTlog)
			activeLog := ctlogutils.ActiveLogStatus(ctlog.Status.Logs)
			if activeLog != nil {
				return activeLog.PublicKey
			}
			return ""
		},
	},
	Fulcio: {
		newInstance:        func() apis.AddressableConditionAware { return &rhtasv1.Fulcio{} },
		materialFromStatus: func(obj apis.AddressableConditionAware) string { return obj.(*rhtasv1.Fulcio).Status.CertificateChain },
	},
	TSA: {
		newInstance: func() apis.AddressableConditionAware { return &rhtasv1.TimestampAuthority{} },
		materialFromStatus: func(obj apis.AddressableConditionAware) string {
			return obj.(*rhtasv1.TimestampAuthority).Status.CertificateChain
		},
	},
}

// Resolve resolves key's address and trust material from binding.
func Resolve(ctx context.Context, cli client.Client, namespace string, key ComponentKey, binding rhtasv1.TrustRootBinding) (Resolved, error) {
	desc, ok := descriptors[key]
	if !ok {
		return Resolved{}, reconcile.TerminalError(fmt.Errorf("%w: %s", ErrUnknownComponent, key))
	}
	return resolveComponent(ctx, cli, namespace, desc, binding, desc.newInstance())
}

// ResolveFulcio resolves Fulcio's address, trust material, and accepted OIDC
// issuers from binding. binding.OIDCIssuers wins if set; otherwise issuers are
// read from the resolved (ref or autodiscovered) Fulcio instance's own config.
func ResolveFulcio(ctx context.Context, cli client.Client, namespace string, binding rhtasv1.TrustRootBindingWithOIDC) (Resolved, error) {
	fulcio := &rhtasv1.Fulcio{}
	resolved, err := resolveComponent(ctx, cli, namespace, descriptors[Fulcio], binding.TrustRootBinding, fulcio)
	if err != nil {
		return Resolved{}, err
	}

	if len(binding.OIDCIssuers) > 0 {
		resolved.OIDCIssuers = binding.OIDCIssuers
		return resolved, nil
	}

	if err := ensureFetched(ctx, cli, namespace, binding.ServiceReference, fulcio); err != nil {
		return Resolved{}, fmt.Errorf("%w: %w", ErrResolveAddress, err)
	}
	for _, oidc := range fulcio.Spec.Config.OIDCIssuers {
		switch {
		case oidc.IssuerURL != "":
			resolved.OIDCIssuers = append(resolved.OIDCIssuers, oidc.IssuerURL)
		case oidc.Issuer != "":
			resolved.OIDCIssuers = append(resolved.OIDCIssuers, oidc.Issuer)
		}
	}
	return resolved, nil
}

func resolveComponent(ctx context.Context, cli client.Client, namespace string, desc descriptor, binding rhtasv1.TrustRootBinding, componentInstance apis.AddressableConditionAware) (Resolved, error) {
	address, err := serviceresolver.ResolveExternalServiceUrl(ctx, cli, binding.ServiceReference, namespace, componentInstance)
	if err != nil {
		return Resolved{}, fmt.Errorf("%w: %w", ErrResolveAddress, err)
	}

	material, err := resolveMaterial(ctx, cli, namespace, desc, binding, componentInstance)
	if err != nil {
		return Resolved{}, err
	}

	return Resolved{Address: address, Material: material}, nil
}

func resolveMaterial(ctx context.Context, cli client.Client, namespace string, desc descriptor, binding rhtasv1.TrustRootBinding, componentInstance apis.AddressableConditionAware) ([]byte, error) {
	if binding.SecretRef != nil {
		data, err := k8sutils.GetSecretData(ctx, cli, namespace, binding.SecretRef)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrResolveMaterial, err)
		}
		if err := trustmaterial.ValidatePEM(data); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrResolveMaterial, err)
		}
		return data, nil
	}

	if err := ensureFetched(ctx, cli, namespace, binding.ServiceReference, componentInstance); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolveMaterial, err)
	}
	if !meta.IsStatusConditionTrue(componentInstance.GetConditions(), constants.ReadyCondition) {
		return nil, fmt.Errorf("%w: %T %s", ErrComponentNotReady, componentInstance, componentInstance.GetName())
	}

	material := []byte(desc.materialFromStatus(componentInstance))
	if err := trustmaterial.ValidatePEM(material); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolveMaterial, err)
	}
	return material, nil
}

// ensureFetched populates instance via ref-or-autoload if it hasn't already been
// populated.
func ensureFetched(ctx context.Context, cli client.Client, namespace string, ref rhtasv1.ServiceReference, instance client.Object) error {
	if instance.GetName() != "" {
		return nil
	}
	return serviceresolver.PopulateInstance(ctx, cli, ref, namespace, instance)
}
