package serviceresolver

import (
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	"github.com/securesign/operator/internal/serviceresolver"
)

func init() {
	serviceresolver.Register(
		func(obj *rhtasv1.Trillian) (string, error) {
			return fmt.Sprintf("dns:///%s.%s.svc:%d", actions.LogserverDeploymentName, obj.Namespace, actions.ServerPort), nil
		})
}
