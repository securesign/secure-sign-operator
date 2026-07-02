package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var rekorlog = logf.Log.WithName("rekor-resource")

type RekorDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-rekor,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=rekors,verbs=create;update,versions=v1,name=mrekor.kb.io,admissionReviewVersions=v1,matchPolicy=Exact

func SetupRekorWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Rekor{}).
		WithDefaulter(&RekorDefaulter{}).
		Complete()
}

func (d *RekorDefaulter) Default(ctx context.Context, obj *Rekor) error {
	rekorlog.Info("default", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
