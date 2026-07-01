package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type CTlogDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-ctlog,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=ctlogs,verbs=create;update,versions=v1,name=mctlog.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupCTlogWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &CTlog{}).
		WithDefaulter(&CTlogDefaulter{}).
		Complete()
}

func (d *CTlogDefaulter) Default(ctx context.Context, obj *CTlog) error {
	logf.FromContext(ctx).WithName("CTlog").Info("setting defaults", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
