package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/action/statusurl"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/utils"
)

type ctlogStatusResolverImpl struct {
}

func (c *ctlogStatusResolverImpl) IsEnabled(instance *rhtasv1.CTlog) bool {
	return utils.IsEnabled(instance.Spec.Ingress.Enabled)
}

func (c *ctlogStatusResolverImpl) GetStatusURL(instance *rhtasv1.CTlog) string {
	return instance.Status.URL
}

func (c *ctlogStatusResolverImpl) SetStatusURL(instance *rhtasv1.CTlog, url string) {
	instance.Status.URL = url
}

func (c *ctlogStatusResolverImpl) GetIngressName() string {
	return DeploymentName
}

func (c *ctlogStatusResolverImpl) GetIngressPath(instance *rhtasv1.CTlog) string {
	return ctlogutils.ActiveLogPrefix(instance.Spec.Logs)
}

func NewStatusUrlAction() action.Action[*rhtasv1.CTlog] {
	return statusurl.NewAction(&ctlogStatusResolverImpl{})
}
