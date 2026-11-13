package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/statusurl"
	tufConstants "github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/utils"
)

type tufStatusResolverImpl struct {
}

func (c *tufStatusResolverImpl) IsEnabled(instance *rhtasv1.Tuf) bool {
	return utils.IsEnabled(instance.Spec.Ingress.Enabled)
}

func (c *tufStatusResolverImpl) GetStatusURL(instance *rhtasv1.Tuf) string {
	return instance.Status.URL
}

func (c *tufStatusResolverImpl) SetStatusURL(instance *rhtasv1.Tuf, url string) {
	instance.Status.URL = url
}

func (c *tufStatusResolverImpl) GetIngressName() string {
	return tufConstants.ComponentName
}

func (c *tufStatusResolverImpl) GetIngressPath(_ *rhtasv1.Tuf) string {
	return ""
}

func NewStatusUrlAction() action.Action[*rhtasv1.Tuf] {
	return statusurl.NewAction(&tufStatusResolverImpl{})
}
