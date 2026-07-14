package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type TufDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-tuf,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=tufs,verbs=create;update,versions=v1,name=mtuf.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupTufWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Tuf{}).
		WithDefaulter(&TufDefaulter{}).
		Complete()
}

func (d *TufDefaulter) Default(ctx context.Context, obj *Tuf) error {
	logf.FromContext(ctx).WithName("Tuf").Info("setting defaults", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
