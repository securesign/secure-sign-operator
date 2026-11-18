package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/statusurl"
	"github.com/securesign/operator/internal/utils"
)

type fulcioStatusResolverImpl struct {
}

func (c *fulcioStatusResolverImpl) IsEnabled(instance *rhtasv1.Fulcio) bool {
	return utils.IsEnabled(instance.Spec.Ingress.Enabled)
}

func (c *fulcioStatusResolverImpl) GetStatusURL(instance *rhtasv1.Fulcio) string {
	return instance.Status.URL
}

func (c *fulcioStatusResolverImpl) SetStatusURL(instance *rhtasv1.Fulcio, url string) {
	instance.Status.URL = url
}

func (c *fulcioStatusResolverImpl) GetIngressName() string {
	return DeploymentName
}

func (c *fulcioStatusResolverImpl) GetIngressPath(_ *rhtasv1.Fulcio) string {
	return ""
}

func NewStatusUrlAction() action.Action[*rhtasv1.Fulcio] {
	return statusurl.NewAction(&fulcioStatusResolverImpl{})
}
