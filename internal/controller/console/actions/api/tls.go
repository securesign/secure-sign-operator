package api

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	tlsAction "github.com/securesign/operator/internal/action/tls"
	"github.com/securesign/operator/internal/controller/console/actions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1.Console] {
	return tlsAction.NewAction(
		actions.ApiCondition,
		metav1.ConditionFalse,
		actions.ApiTLSSecret,
		"console api",
		tlsAction.Wrapper(specTLS, statusTLS, setStatusTLS, nil),
	)
}
