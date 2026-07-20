package actions

import (
	"context"
	"net/url"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type tsaTrustMaterialResolver struct{}

func (r tsaTrustMaterialResolver) ComponentName() string { return ComponentName }

func (r tsaTrustMaterialResolver) CanHandle(_ context.Context, instance *rhtasv1.TimestampAuthority) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
}

func (r tsaTrustMaterialResolver) GetTrustMaterial(instance *rhtasv1.TimestampAuthority) string {
	return instance.Status.CertificateChain
}

func (r tsaTrustMaterialResolver) SetTrustMaterial(instance *rhtasv1.TimestampAuthority, pem string) {
	instance.Status.CertificateChain = pem
}

func (r tsaTrustMaterialResolver) Resolve(ctx context.Context, cli client.Client, instance *rhtasv1.TimestampAuthority) ([]byte, error) {
	baseURL := trustmaterial.ResolveBaseURL(DeploymentName, instance.Namespace, instance.Status.Url, ServerPort)
	u, err := url.JoinPath(baseURL, TimestampPath, "certchain")
	if err != nil {
		return nil, err
	}
	return trustmaterial.FetchPEMOverHTTP(ctx, cli, instance, u)
}

func NewResolvePubKeyAction() action.Action[*rhtasv1.TimestampAuthority] {
	return trustmaterial.NewAction[*rhtasv1.TimestampAuthority](tsaTrustMaterialResolver{})
}
