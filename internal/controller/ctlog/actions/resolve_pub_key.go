package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	k8sutils "github.com/securesign/operator/internal/utils/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrPublicKeyRefNotSet = errors.New("PublicKeyRef not set in status")
	ErrSecretRead         = errors.New("failed to read public key from secret")
)

type ctlogTrustMaterialResolver struct{}

func (r ctlogTrustMaterialResolver) ComponentName() string { return ComponentName }

func (r ctlogTrustMaterialResolver) ConditionType() string { return constants.ReadyCondition }

func (r ctlogTrustMaterialResolver) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
}

func (r ctlogTrustMaterialResolver) GetTrustMaterial(instance *rhtasv1.CTlog) string {
	return instance.Status.PublicKey
}

func (r ctlogTrustMaterialResolver) SetTrustMaterial(instance *rhtasv1.CTlog, pem string) {
	instance.Status.PublicKey = pem
}

func (r ctlogTrustMaterialResolver) Resolve(_ context.Context, cli client.Client, instance *rhtasv1.CTlog) ([]byte, error) {
	if instance.Status.PublicKeyRef == nil {
		return nil, fmt.Errorf("%w: ctlog", ErrPublicKeyRefNotSet)
	}
	data, err := k8sutils.GetSecretData(cli, instance.Namespace, instance.Status.PublicKeyRef)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSecretRead, instance.Status.PublicKeyRef.Name, err)
	}
	return data, nil
}

func NewResolvePubKeyAction() action.Action[*rhtasv1.CTlog] {
	return trustmaterial.NewAction[*rhtasv1.CTlog](ctlogTrustMaterialResolver{})
}
