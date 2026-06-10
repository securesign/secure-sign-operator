package db

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	tlsAction "github.com/securesign/operator/internal/action/tls"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1.Trillian] {
	return tlsAction.NewAction(
		actions.DbCondition,
		metav1.ConditionFalse,
		actions.DatabaseTLSSecret,
		"trillian database",
		tlsAction.Wrapper(specTLS, statusTLS, setStatusTLS, enabled),
	)
}
