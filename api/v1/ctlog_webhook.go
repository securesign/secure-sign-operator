package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var ctloglog = logf.Log.WithName("ctlog-resource")

// CTlogDefaulter is a no-op scaffold; real defaulting logic will be added in SECURESIGN-4581.
type CTlogDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-ctlog,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=ctlogs,verbs=create;update,versions=v1,name=mctlog.kb.io,admissionReviewVersions=v1

func SetupCTlogWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &CTlog{}).
		WithDefaulter(&CTlogDefaulter{}).
		Complete()
}

func (d *CTlogDefaulter) Default(_ context.Context, obj *CTlog) error {
	ctloglog.Info("default", "name", obj.Name)
	return nil
}
