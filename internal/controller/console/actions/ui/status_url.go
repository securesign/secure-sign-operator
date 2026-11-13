package ui

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/statusurl"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/utils"
)

type consoleStatusResolverImpl struct {
}

func (c *consoleStatusResolverImpl) IsEnabled(instance *rhtasv1.Console) bool {
	return utils.IsEnabled(instance.Spec.UI.Ingress.Enabled)
}

func (c *consoleStatusResolverImpl) GetStatusURL(instance *rhtasv1.Console) string {
	return instance.Status.UI.URL
}

func (c *consoleStatusResolverImpl) SetStatusURL(instance *rhtasv1.Console, url string) {
	instance.Status.UI.URL = url
}

func (c *consoleStatusResolverImpl) GetIngressName() string {
	return actions.UIDeploymentName
}

func (c *consoleStatusResolverImpl) GetIngressPath(_ *rhtasv1.Console) string {
	return ""
}

func NewStatusUrlAction() action.Action[*rhtasv1.Console] {

	return statusurl.NewAction(&consoleStatusResolverImpl{})
}
