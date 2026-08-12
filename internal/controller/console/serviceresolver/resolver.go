package serviceresolver

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/serviceresolver"
)

func init() {
	serviceresolver.Register(
		func(obj *rhtasv1.Console) (string, error) {
			return fmt.Sprintf("http://%s.%s.svc", actions.UIDeploymentName, obj.Namespace), nil
		})
}
