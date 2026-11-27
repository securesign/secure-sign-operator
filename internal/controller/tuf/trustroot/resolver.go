// Package trustroot resolves configured component bindings for TUF publication.
package trustroot

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/apis"
	"github.com/securesign/operator/internal/constants"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/serviceresolver"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var componentSuppliers = map[ComponentKey]func() apis.AddressableConditionAware{
	Rekor:  func() apis.AddressableConditionAware { return &rhtasv1.Rekor{} },
	CTFE:   func() apis.AddressableConditionAware { return &rhtasv1.CTlog{} },
	Fulcio: func() apis.AddressableConditionAware { return &rhtasv1.Fulcio{} },
	TSA:    func() apis.AddressableConditionAware { return &rhtasv1.TimestampAuthority{} },
}

type TrustRootBinder interface {
	apis.ServiceReferencer
	GetSecretRef() *rhtasv1.SecretKeySelector
	GetValidFor() *rhtasv1.ValidFor
	GetName() string
}

// ResolvedBinding holds the publication inputs for a TUF target.
type ResolvedBinding struct {
	BaseURL       string           `json:"uri"`
	TrustMaterial string           `json:"material"`
	ValidFor      ResolvedValidity `json:"validFor"`
	OIDCIssuers   []string         `json:"oidcIssuers,omitempty"`
}

// ResolvedValidity holds the optional bounds of a target's validity interval.
type ResolvedValidity struct {
	Start *metav1.Time `json:"start"`
	End   *metav1.Time `json:"end"`
}

// ResolvedComponents holds the resolved bindings indexed by TUF target filename.
type ResolvedComponents struct {
	Ctlog  map[string]ResolvedBinding `json:"ctlog"`
	Fulcio map[string]ResolvedBinding `json:"fulcio"`
	Rekor  map[string]ResolvedBinding `json:"rekor"`
	Tsa    map[string]ResolvedBinding `json:"tsa"`
}

// ResolvedTrustRootDocument holds the resolved component maps used to generate a trust root patch.
type ResolvedTrustRootDocument struct {
	Components ResolvedComponents `json:"components"`
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

// ResolveBinding resolves the address and trust material for a component binding.
func ResolveBinding(ctx context.Context, cli client.Client, namespace string, component ComponentKey, b TrustRootBinder) (ResolvedBinding, error) {
	if err := validateExplicitBinding(b); err != nil {
		return ResolvedBinding{}, err
	}
	componentInstanceSupplier, ok := componentSuppliers[component]
	if !ok {
		return ResolvedBinding{}, fmt.Errorf("no component supplier found for %q", component)
	}
	componentInstance := componentInstanceSupplier()

	address, err := serviceresolver.ResolveExternalServiceUrl(ctx, cli, b.GetServiceRef(), namespace, componentInstance)
	if err != nil {
		return ResolvedBinding{}, fmt.Errorf("%w: %w", ErrResolveAddress, err)
	}

	material, err := resolveMaterial(ctx, cli, namespace, b, componentInstance)
	if err != nil {
		return ResolvedBinding{}, err
	}

	issuers := []string{}
	if withOidc, ok := any(b).(rhtasv1.TrustRootBindingWithOIDC); ok {
		issuers = withOidc.OIDCIssuers
	}

	resolved := ResolvedBinding{
		BaseURL:       address,
		TrustMaterial: string(material),
		OIDCIssuers:   issuers,
	}
	if validFor := b.GetValidFor(); validFor != nil {
		resolved.ValidFor = ResolvedValidity{
			Start: validFor.Start.DeepCopy(),
			End:   validFor.End.DeepCopy(),
		}
	}

	if fulcio, ok := componentInstance.(*rhtasv1.Fulcio); ok && len(issuers) == 0 {
		if err := ensureFetched(ctx, cli, namespace, b.GetServiceRef(), fulcio); err != nil {
			return ResolvedBinding{}, fmt.Errorf("%w: %w", ErrResolveAddress, err)
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

// Resolve resolves all configured component bindings without discovering
// component resources by listing them.
func Resolve(ctx context.Context, cli client.Client, namespace string, source rhtasv1.Components) (ResolvedTrustRootDocument, error) {
	var components ResolvedComponents
	var err error
	if components.Rekor, err = resolveBindings(ctx, cli, namespace, Rekor, source.Rekor); err != nil {
		return ResolvedTrustRootDocument{}, err
	}
	if components.Ctlog, err = resolveBindings(ctx, cli, namespace, CTFE, source.Ctlog); err != nil {
		return ResolvedTrustRootDocument{}, err
	}
	if components.Fulcio, err = resolveBindings(ctx, cli, namespace, Fulcio, source.Fulcio); err != nil {
		return ResolvedTrustRootDocument{}, err
	}
	if components.Tsa, err = resolveBindings(ctx, cli, namespace, TSA, source.Tsa); err != nil {
		return ResolvedTrustRootDocument{}, err
	}
	return ResolvedTrustRootDocument{Components: components}, nil
}

func resolveBindings[T TrustRootBinder](ctx context.Context, cli client.Client, namespace string, component ComponentKey, bindings []T) (map[string]ResolvedBinding, error) {
	resolved := make(map[string]ResolvedBinding, len(bindings))
	for i, binding := range bindings {
		target := cmp.Or(binding.GetName(), string(component))
		if _, exists := resolved[target]; exists {
			return nil, fmt.Errorf("duplicate TUF target %q for %s", target, component)
		}
		entry, err := ResolveBinding(ctx, cli, namespace, component, binding)
		if err != nil {
			return nil, fmt.Errorf("resolve %s binding %d: %w", component, i, err)
		}
		resolved[target] = entry
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
