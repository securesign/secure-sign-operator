package actions

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	tlsAction "github.com/securesign/operator/internal/action/tls"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewTlsAction() action.Action[*rhtasv1.CTlog] {
	return tlsAction.NewAction(
		TLSCondition,
		metav1.ConditionTrue,
		TLSSecret,
		"CTLog",
		tlsAction.Wrapper(
			func(c *rhtasv1.CTlog) rhtasv1.TLS { return c.Spec.TLS },
			func(c *rhtasv1.CTlog) rhtasv1.TLS { return c.Status.TLS },
			func(c *rhtasv1.CTlog, tls rhtasv1.TLS) { c.Status.TLS = tls },
			nil,
		),
	)
}
