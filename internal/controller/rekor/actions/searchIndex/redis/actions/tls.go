package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	tlsAction "github.com/securesign/operator/internal/action/tls"
	actions2 "github.com/securesign/operator/internal/controller/rekor/actions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1.Rekor] {
	return tlsAction.NewAction(
		actions2.RedisCondition,
		metav1.ConditionFalse,
		actions2.RedisTlsSecret,
		"redis server",
		tlsAction.Wrapper(specTLS, statusTLS, setStatusTLS, enabled),
	)
}
