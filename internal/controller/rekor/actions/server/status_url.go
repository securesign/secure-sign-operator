package server

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/statusurl"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/utils"
)

type rekorStatusResolverImpl struct {
}

func (c *rekorStatusResolverImpl) IsEnabled(instance *rhtasv1.Rekor) bool {
	return utils.IsEnabled(instance.Spec.Ingress.Enabled)
}

func (c *rekorStatusResolverImpl) GetStatusURL(instance *rhtasv1.Rekor) string {
	return instance.Status.URL
}

func (c *rekorStatusResolverImpl) SetStatusURL(instance *rhtasv1.Rekor, url string) {
	instance.Status.URL = url
}

func (c *rekorStatusResolverImpl) GetIngressName() string {
	return actions.ServerDeploymentName
}

func (c *rekorStatusResolverImpl) GetIngressPath(_ *rhtasv1.Rekor) string {
	return ""
}

func NewStatusUrlAction() action.Action[*rhtasv1.Rekor] {
	return statusurl.NewAction(&rekorStatusResolverImpl{})
}
