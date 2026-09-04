package actions

import (
	"context"
	"errors"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
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

func (r ctlogTrustMaterialResolver) CanHandle(_ context.Context, instance *rhtasv1.CTlog) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
}

func (r ctlogTrustMaterialResolver) GetTrustMaterial(instance *rhtasv1.CTlog) string {
	activeLog := getActiveLogStatus(instance)
	if activeLog == nil {
		return ""
	}
	return activeLog.PublicKey
}

func (r ctlogTrustMaterialResolver) SetTrustMaterial(instance *rhtasv1.CTlog, pem string) {
	activeLog := getActiveLogStatus(instance)
	if activeLog == nil {
		return
	}
	activeLog.PublicKey = pem
}

func (r ctlogTrustMaterialResolver) Resolve(ctx context.Context, cli client.Client, instance *rhtasv1.CTlog) ([]byte, error) {
	activeLog := getActiveLogStatus(instance)
	if activeLog == nil || activeLog.PublicKeyRef == nil {
		return nil, fmt.Errorf("%w: ctlog", ErrPublicKeyRefNotSet)
	}
	data, err := k8sutils.GetSecretData(ctx, cli, instance.Namespace, activeLog.PublicKeyRef)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrSecretRead, activeLog.PublicKeyRef.Name, err)
	}
	return data, nil
}

func NewResolvePubKeyAction() action.Action[*rhtasv1.CTlog] {
	return trustmaterial.NewAction[*rhtasv1.CTlog](ctlogTrustMaterialResolver{})
}

func getActiveLogStatus(instance *rhtasv1.CTlog) *rhtasv1.CTlogLogStatus {
	return utils.ActiveLogStatus(instance.Status.Logs)
}
