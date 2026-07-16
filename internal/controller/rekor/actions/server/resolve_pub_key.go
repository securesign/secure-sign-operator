package server

import (
	"context"
	"net/url"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/trustmaterial"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/state"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type rekorTrustMaterialResolver struct{}

func (r rekorTrustMaterialResolver) ComponentName() string { return actions.ServerComponentName }

func (r rekorTrustMaterialResolver) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return state.FromInstance(instance, actions.ServerCondition) >= state.Initialize
}

func (r rekorTrustMaterialResolver) GetTrustMaterial(instance *rhtasv1.Rekor) string {
	return instance.Status.PublicKey
}

func (r rekorTrustMaterialResolver) SetTrustMaterial(instance *rhtasv1.Rekor, pem string) {
	instance.Status.PublicKey = pem
}

func (r rekorTrustMaterialResolver) Resolve(ctx context.Context, cli client.Client, instance *rhtasv1.Rekor) ([]byte, error) {
	baseURL := trustmaterial.ResolveBaseURL(actions.ServerDeploymentName, instance.Namespace, instance.Status.Url)
	u, err := url.JoinPath(baseURL, "/api/v1/log/publicKey")
	if err != nil {
		return nil, err
	}
	return trustmaterial.FetchPEMOverHTTP(ctx, cli, instance, u)
}

func NewResolvePubKeyAction() action.Action[*rhtasv1.Rekor] {
	return trustmaterial.NewAction[*rhtasv1.Rekor](rekorTrustMaterialResolver{})
}
