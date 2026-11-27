package actions

import (
	"context"
	"fmt"
	"maps"
	"slices"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFIPSValidationAction() action.Action[*rhtasv1.Tuf] {
	return fipsAction.NewAction(
		fipsutil.FIPSCondition,
		tufConstants.ComponentName,
		fipsAction.Wrapper(fipsAction.Config[*rhtasv1.Tuf]{
			CryptoMaterial: tufCryptoMaterial,
		}),
	)
}

func tufCryptoMaterial(ctx context.Context, i *rhtasv1.Tuf, c client.Client) ([]fipsAction.CryptoRef, error) {
	var refs []fipsAction.CryptoRef
	asTrustRootBindings := func(bindings []rhtasv1.TrustRootBindingWithOIDC) []rhtasv1.TrustRootBinding {
		b := make([]rhtasv1.TrustRootBinding, len(bindings))
		for i, binding := range bindings {
			b[i] = binding.TrustRootBinding
		}
		return b
	}

	components := map[string][]rhtasv1.TrustRootBinding{
		"ctlog":  i.Spec.Ctlog,
		"fulcio": asTrustRootBindings(i.Spec.Fulcio),
		"rekor":  i.Spec.Rekor,
		"tsa":    ptr.Deref(i.Spec.Tsa, nil),
	}
	for component, array := range components {
		for _, key := range array {

			if key.SecretRef == nil {
				continue
			}
			if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, key.SecretRef,
				fmt.Sprintf("spec.%s[0].secretRef", component), fipsutil.ValidateCryptoMaterialPEM, &refs); err != nil {
				return nil, err
			}
		}
	}

	for _, ks := range i.Status.Keys {
		if slices.ContainsFunc(slices.Concat(slices.Collect(maps.Values(components))...), func(key rhtasv1.TrustRootBinding) bool {
			return key.SecretRef != nil && key.SecretRef.Name == ks.SecretRef.Name
		}) {
			continue
		}
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, ks.SecretRef,
			fmt.Sprintf("status.keys[%s] (autodiscovered)", ks.Name), fipsutil.ValidateCryptoMaterialPEM, &refs); err != nil {
			return nil, err
		}
	}

	return refs, nil
}
