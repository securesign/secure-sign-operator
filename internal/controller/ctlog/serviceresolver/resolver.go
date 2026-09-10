package serviceresolver

import (
	"fmt"
	"net/url"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/controller/ctlog/actions"
	ctlogutils "github.com/securesign/operator/internal/controller/ctlog/utils"
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
			if ctlogutils.TlsEnabled(obj) {

				protocol = "https"
			} else {
				protocol = "http"
			}
			activeLog := ctlogutils.ActiveLogStatus(obj.Status.Logs)
			if activeLog == nil || activeLog.Prefix == "" {
				return "", fmt.Errorf("no active shard or prefix found in CTLog status")
			}
			u := url.URL{
				Scheme: protocol,
				Host:   fmt.Sprintf("%s.%s.svc", actions.DeploymentName, obj.Namespace),
				Path:   activeLog.Prefix,
			}
			return u.String(), nil
		})
}
