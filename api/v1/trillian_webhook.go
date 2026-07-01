package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type TrillianDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-trillian,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=trillians,verbs=create;update,versions=v1,name=mtrillian.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupTrillianWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Trillian{}).
		WithDefaulter(&TrillianDefaulter{}).
		Complete()
}

func (d *TrillianDefaulter) Default(ctx context.Context, obj *Trillian) error {
	logf.FromContext(ctx).WithName("Trillian").Info("setting defaults", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
