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

type fulcioTrustMaterialResolver struct{}

func (r fulcioTrustMaterialResolver) ComponentName() string { return ComponentName }

func (r fulcioTrustMaterialResolver) CanHandle(_ context.Context, instance *rhtasv1.Fulcio) bool {
	return state.FromInstance(instance, constants.ReadyCondition) >= state.Initialize
}

func (r fulcioTrustMaterialResolver) GetTrustMaterial(instance *rhtasv1.Fulcio) string {
	return instance.Status.CertificateChain
}

func (r fulcioTrustMaterialResolver) SetTrustMaterial(instance *rhtasv1.Fulcio, pem string) {
	instance.Status.CertificateChain = pem
}

func (r fulcioTrustMaterialResolver) Resolve(ctx context.Context, cli client.Client, instance *rhtasv1.Fulcio) ([]byte, error) {
	baseURL := trustmaterial.ResolveBaseURL(DeploymentName, instance.Namespace, instance.Status.Url)
	u, err := url.JoinPath(baseURL, "/api/v2/trustBundle")
	if err != nil {
		return nil, err
	}
	body, err := trustmaterial.FetchPEMOverHTTP(ctx, cli, instance, u)
	if err != nil {
		return nil, err
	}
	return trustmaterial.ParseTrustBundle(body)
}

func NewResolvePubKeyAction() action.Action[*rhtasv1.Fulcio] {
	return trustmaterial.NewAction[*rhtasv1.Fulcio](fulcioTrustMaterialResolver{})
}
