package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/statusurl"
	"github.com/securesign/operator/internal/utils"
)

type tsaStatusResolverImpl struct {
}

func (c *tsaStatusResolverImpl) IsEnabled(instance *rhtasv1.TimestampAuthority) bool {
	return utils.IsEnabled(instance.Spec.Ingress.Enabled)
}

func (c *tsaStatusResolverImpl) GetStatusURL(instance *rhtasv1.TimestampAuthority) string {
	return instance.Status.URL
}

func (c *tsaStatusResolverImpl) SetStatusURL(instance *rhtasv1.TimestampAuthority, url string) {
	instance.Status.URL = url
}

func (c *tsaStatusResolverImpl) GetIngressName() string {
	return DeploymentName
}

func (c *tsaStatusResolverImpl) GetIngressPath(_ *rhtasv1.TimestampAuthority) string {
	return rhtasv1.TimestampPath
}

func NewStatusUrlAction() action.Action[*rhtasv1.TimestampAuthority] {
	return statusurl.NewAction(&tsaStatusResolverImpl{})
}
