package serviceresolver

import (
	"fmt"
	"net/url"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	"github.com/securesign/operator/internal/controller/ctlog/utils"
	"github.com/securesign/operator/internal/serviceresolver"
	"k8s.io/apimachinery/pkg/api/meta"
)

func init() {
	serviceresolver.Register(
		func(obj *rhtasv1.CTlog) (string, error) {
			if !meta.IsStatusConditionTrue(obj.Status.Conditions, actions.TLSCondition) {
				return "", fmt.Errorf("TLS is not yet resolved")
			}
			var protocol string
			if utils.TlsEnabled(obj) {

				protocol = "https"
			} else {
				protocol = "http"
			}
			prefix := ""
			if len(obj.Spec.Logs) > 0 {
				prefix = obj.Spec.Logs[0].Prefix
			}
			u := url.URL{
				Scheme: protocol,
				Host:   fmt.Sprintf("%s.%s.svc", actions.DeploymentName, obj.Namespace),
				Path:   prefix,
			}
			return u.String(), nil
		})
}
