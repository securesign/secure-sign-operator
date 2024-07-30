package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ReconcilePanics is a prometheus counter metrics which holds the total
	// number of panic from the Reconciler.
	ReconcilePanics = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "controller_runtime_reconcile_panics_total",
		Help: "Total number of reconciliation panics per controller",
	})
)

// init will register metrics with the global prometheus registry
func init() {
	metrics.Registry.MustRegister(ReconcilePanics)
}
