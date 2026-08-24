package actions

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	fipsAction "github.com/securesign/operator/internal/action/fips"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/controller/tuf/trustroot"
	fipsutil "github.com/securesign/operator/internal/utils/fips"
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

	specHasRef := make(map[string]bool)
	for _, key := range trustroot.ActiveKeys(i) {
		secretRef := trustroot.Binding(i, key).SecretRef
		if secretRef == nil {
			continue
		}
		specHasRef[key.String()] = true
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, secretRef,
			fmt.Sprintf("spec.%s[0].secretRef", key), fipsutil.ValidateCryptoMaterialPEM, &refs); err != nil {
			return nil, err
		}
	}

	for _, ks := range i.Status.Keys {
		if ks.SecretRef == nil || specHasRef[ks.Name] {
			continue
		}
		if err := fipsAction.AppendSecretRef(ctx, c, i.Namespace, ks.SecretRef,
			fmt.Sprintf("status.keys[%s] (autodiscovered)", ks.Name), fipsutil.ValidateCryptoMaterialPEM, &refs); err != nil {
			return nil, err
		}
	}

	return refs, nil
}
