package serviceresolver

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/tsa/actions"
	"github.com/securesign/operator/internal/serviceresolver"
)

func init() {
	serviceresolver.Register(
		func(obj *rhtasv1.TimestampAuthority) (string, error) {
			return fmt.Sprintf("http://%s.%s.svc:%d%s", actions.DeploymentName, obj.Namespace, actions.ServerPort, rhtasv1.TimestampPath), nil
		})
}
