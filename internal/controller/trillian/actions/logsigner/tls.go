package logsigner

import (
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	tlsAction "github.com/securesign/operator/internal/action/tls"
	"github.com/securesign/operator/internal/controller/trillian/actions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1alpha1.Trillian] {
	return tlsAction.NewAction(
		actions.SignerCondition,
		metav1.ConditionFalse,
		actions.LogSignerTLSSecret,
		"trillian log signer",
		tlsAction.Wrapper(specTLS, statusTLS, setStatusTLS, nil),
	)
}
