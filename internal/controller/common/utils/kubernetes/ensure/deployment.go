package ensure

import (
	"github.com/securesign/operator/internal/controller/common/utils"
	v1 "k8s.io/api/apps/v1"
)

func Proxy() func(*v1.Deployment) error {
	return func(dp *v1.Deployment) error {
		utils.SetProxyEnvs(dp)
		return nil
	}
}
