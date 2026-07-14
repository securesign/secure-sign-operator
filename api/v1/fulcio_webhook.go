package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type FulcioDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-fulcio,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=fulcios,verbs=create;update,versions=v1,name=mfulcio.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupFulcioWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Fulcio{}).
		WithDefaulter(&FulcioDefaulter{}).
		Complete()
}

func (d *FulcioDefaulter) Default(ctx context.Context, obj *Fulcio) error {
	logf.FromContext(ctx).WithName("Fulcio").Info("setting defaults", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
