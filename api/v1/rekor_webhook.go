package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type RekorDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-rekor,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=rekors,verbs=create;update,versions=v1,name=mrekor.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupRekorWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Rekor{}).
		WithDefaulter(&RekorDefaulter{}).
		Complete()
}

func (d *RekorDefaulter) Default(ctx context.Context, obj *Rekor) error {
	logf.FromContext(ctx).WithName("Rekor").Info("setting defaults", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
