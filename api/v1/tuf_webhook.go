package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var tuflog = logf.Log.WithName("tuf-resource")

type TufDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-tuf,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=tufs,verbs=create;update,versions=v1,name=mtuf.kb.io,admissionReviewVersions=v1,matchPolicy=Exact

func SetupTufWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Tuf{}).
		WithDefaulter(&TufDefaulter{}).
		Complete()
}

func (d *TufDefaulter) Default(ctx context.Context, obj *Tuf) error {
	tuflog.Info("default", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
