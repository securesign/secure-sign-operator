package serviceresolver

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/tuf/constants"
	"github.com/securesign/operator/internal/serviceresolver"
)

func init() {
	serviceresolver.Register(
		func(obj *rhtasv1.Tuf) (string, error) {
			return fmt.Sprintf("http://%s.%s.svc:%d", constants.DeploymentName, obj.Namespace, obj.Spec.Port), nil
		})
}
