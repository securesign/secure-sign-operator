// Package trustroot resolves configured component bindings for TUF publication.
package trustroot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/serviceresolver"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ComponentKey identifies a trust root component by its well-known TUF target filename.
type ComponentKey string

const (
	Rekor  ComponentKey = ComponentKey(rhtasv1.TufKeyRekor)
	CTFE   ComponentKey = ComponentKey(rhtasv1.TufKeyCTFE)
	Fulcio ComponentKey = ComponentKey(rhtasv1.TufKeyFulcio)
	TSA    ComponentKey = ComponentKey(rhtasv1.TufKeyTSA)
)

var (
	ErrResolveAddress    = errors.New("failed to resolve component address")
	ErrResolveMaterial   = errors.New("failed to resolve trust material")
	ErrComponentNotReady = errors.New("component instance not ready")
)

type TrustRootBinder interface {
	apis.ServiceReferencer
	GetSecretRef() *rhtasv1.SecretKeySelector
	GetValidFor() *rhtasv1.ValidFor
	GetName() string
}

// ResolvedBinding holds a binding and its resolved publication inputs, including the PEM bytes.
type ResolvedBinding struct {
	BaseURL       string           `json:"baseURL"`
	TrustMaterial []byte           `json:"trustMaterial"`
	ValidFor      rhtasv1.ValidFor `json:"validFor"`
	OIDCIssuers   []string         `json:"oidcIssuers,omitempty"`
}

// ResolvedComponents holds the resolved bindings for each trust root component.
type ResolvedComponents struct {
	Ctlog  []ResolvedBinding
	Fulcio []ResolvedBinding
	Rekor  []ResolvedBinding
	Tsa    []ResolvedBinding
}

var materialFromStatus = map[reflect.Type]func(apis.AddressableConditionAware) string{
	reflect.TypeOf(&rhtasv1.Rekor{}): func(obj apis.AddressableConditionAware) string { return obj.(*rhtasv1.Rekor).Status.PublicKey },
	reflect.TypeOf(&rhtasv1.CTlog{}): func(obj apis.AddressableConditionAware) string {
		ctlog := obj.(*rhtasv1.CTlog)
		activeLog := ctlogutils.ActiveLogStatus(ctlog.Status.Logs)
		if activeLog != nil {
			return activeLog.PublicKey
		}
		return ""
	},
	reflect.TypeOf(&rhtasv1.Fulcio{}): func(obj apis.AddressableConditionAware) string { return obj.(*rhtasv1.Fulcio).Status.CertificateChain },
	reflect.TypeOf(&rhtasv1.TimestampAuthority{}): func(obj apis.AddressableConditionAware) string {
		return obj.(*rhtasv1.TimestampAuthority).Status.CertificateChain
	},
}

func resolveBinding(ctx context.Context, cli client.Client, namespace string, b TrustRootBinder, componentInstance apis.AddressableConditionAware, issuers ...string) (ResolvedBinding, error) {
	if err := validateExplicitBinding(b); err != nil {
		return ResolvedBinding{}, err
	}

	address, err := serviceresolver.ResolveExternalServiceUrl(ctx, cli, b.GetServiceRef(), namespace, componentInstance)
	if err != nil {
		return ResolvedBinding{}, fmt.Errorf("%w: %w", ErrResolveAddress, err)
	}

	material, err := resolveMaterial(ctx, cli, namespace, b, componentInstance)
	if err != nil {
		return ResolvedBinding{}, err
	}

	resolved := ResolvedBinding{
		BaseURL:       address,
		TrustMaterial: material,
		OIDCIssuers:   slices.Clone(issuers),
	}
	if b.GetValidFor() != nil {
		resolved.ValidFor = rhtasv1.ValidFor{
			Start: b.GetValidFor().Start.DeepCopy(),
			End:   b.GetValidFor().End.DeepCopy(),
		}
	}
	if fulcio, ok := componentInstance.(*rhtasv1.Fulcio); ok && len(issuers) == 0 {
		if err := ensureFetched(ctx, cli, namespace, b.GetServiceRef(), fulcio); err != nil {
			return ResolvedBinding{}, fmt.Errorf("%w: %w", ErrResolveAddress, err)
		}
		for _, oidc := range fulcio.Spec.Config.OIDCIssuers {
			issuer := oidc.IssuerURL
			if issuer == "" {
				issuer = oidc.Issuer
			}
			if issuer != "" {
				resolved.OIDCIssuers = append(resolved.OIDCIssuers, issuer)
			}
		}
	}
	return resolved, nil
}

func resolveMaterial(ctx context.Context, cli client.Client, namespace string, b TrustRootBinder, componentInstance apis.AddressableConditionAware) ([]byte, error) {
	if b.GetSecretRef() != nil {
		data, err := k8sutils.GetSecretData(ctx, cli, namespace, b.GetSecretRef())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrResolveMaterial, err)
		}
		if err := trustmaterial.ValidatePEM(data); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrResolveMaterial, err)
		}
		return data, nil
	}

	if err := ensureFetched(ctx, cli, namespace, b.GetServiceRef(), componentInstance); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolveMaterial, err)
	}
	if !meta.IsStatusConditionTrue(componentInstance.GetConditions(), constants.ReadyCondition) {
		return nil, fmt.Errorf("%w: %T %s", ErrComponentNotReady, componentInstance, componentInstance.GetName())
	}

	materialFromStatus, ok := materialFromStatus[reflect.TypeOf(componentInstance)]
	if !ok {
		return nil, fmt.Errorf("%w: %T", ErrResolveMaterial, componentInstance)
	}

	material := []byte(materialFromStatus(componentInstance))
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

// ResolveFromSpec resolves all configured component bindings without discovering
// component resources by listing them.
func Resolve(ctx context.Context, cli client.Client, namespace string, source rhtasv1.Components) (ResolvedComponents, error) {

	resolveWrapper := func(instance apis.AddressableConditionAware, b TrustRootBinder, issuers ...string) (ResolvedBinding, error) {
		return resolveBinding(ctx, cli, namespace, b, instance, issuers...)
	}
	var components ResolvedComponents
	var err error
	if components.Rekor, err = resolveBindings(func() apis.AddressableConditionAware { return &rhtasv1.Rekor{} }, source.Rekor, resolveWrapper); err != nil {
		return ResolvedComponents{}, err
	}
	if components.Ctlog, err = resolveBindings(func() apis.AddressableConditionAware { return &rhtasv1.CTlog{} }, source.Ctlog, resolveWrapper); err != nil {
		return ResolvedComponents{}, err
	}
	if components.Fulcio, err = resolveBindings(func() apis.AddressableConditionAware { return &rhtasv1.Fulcio{} }, source.Fulcio, resolveWrapper); err != nil {
		return ResolvedComponents{}, err
	}
	if components.Tsa, err = resolveBindings(func() apis.AddressableConditionAware { return &rhtasv1.TimestampAuthority{} }, source.Tsa, resolveWrapper); err != nil {
		return ResolvedComponents{}, err
	}
	return components, nil
}

func resolveBindings[T TrustRootBinder](newInstanceSupplier func() apis.AddressableConditionAware, bindings []T, resolve func(apis.AddressableConditionAware, TrustRootBinder, ...string) (ResolvedBinding, error)) ([]ResolvedBinding, error) {
	resolved := make([]ResolvedBinding, len(bindings))
	for i, binding := range bindings {
		var issuers []string
		if withOidc, ok := any(binding).(rhtasv1.TrustRootBindingWithOIDC); ok {
			issuers = withOidc.OIDCIssuers
		}
		newInstance := newInstanceSupplier()
		entry, err := resolve(newInstance, binding, issuers...)
		if err != nil {
			return nil, fmt.Errorf("resolve %T binding %d: %w", newInstance, i, err)
		}
		resolved[i] = entry
	}
	return resolved, nil
}

func validateExplicitBinding(b TrustRootBinder) error {
	if hasServiceRef(b.GetServiceRef().Ref) {
		return nil
	}
	address, err := url.Parse(b.GetServiceRef().URL)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrResolveAddress, err)
	}
	if address.Hostname() == "" {
		return fmt.Errorf("%w: binding requires ref or a URL with a hostname", ErrResolveAddress)
	}
	if b.GetSecretRef() == nil {
		return fmt.Errorf("%w: binding requires ref or secretRef", ErrResolveMaterial)
	}
	return nil
}

func hasServiceRef(ref *rhtasv1.ServiceReferenceRef) bool {
	return ref != nil && ref.Name != ""
}
