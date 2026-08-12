package serviceresolver

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	"github.com/securesign/operator/internal/serviceresolver"
)

func init() {
	serviceresolver.Register(
		func(obj *rhtasv1.Rekor) (string, error) {
			return fmt.Sprintf("http://%s.%s.svc", actions.ServerDeploymentName, obj.Namespace), nil
		})
}
