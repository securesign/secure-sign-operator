package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type SecuresignDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-securesign,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=securesigns,verbs=create;update,versions=v1,name=msecuresign.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupSecuresignWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Securesign{}).
		WithDefaulter(&SecuresignDefaulter{}).
		Complete()
}

func (d *SecuresignDefaulter) Default(ctx context.Context, obj *Securesign) error {
	logf.FromContext(ctx).WithName("Securesign").Info("setting defaults", "name", obj.Name)
	obj.SetDefaults()
	return nil
}
