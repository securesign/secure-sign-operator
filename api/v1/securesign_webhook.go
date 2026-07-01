package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var securesignlog = logf.Log.WithName("securesign-resource")

type SecuresignDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-securesign,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=securesigns,verbs=create;update,versions=v1,name=msecuresign.kb.io,admissionReviewVersions=v1,matchPolicy=Exact

func SetupSecuresignWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Securesign{}).
		WithDefaulter(&SecuresignDefaulter{}).
		Complete()
}

func (d *SecuresignDefaulter) Default(ctx context.Context, obj *Securesign) error {
	securesignlog.Info("default", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
