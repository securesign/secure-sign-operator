package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var fulciolog = logf.Log.WithName("fulcio-resource")

type FulcioDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-fulcio,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=fulcios,verbs=create;update,versions=v1,name=mfulcio.kb.io,admissionReviewVersions=v1,matchPolicy=Exact

func SetupFulcioWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Fulcio{}).
		WithDefaulter(&FulcioDefaulter{}).
		Complete()
}

func (d *FulcioDefaulter) Default(ctx context.Context, obj *Fulcio) error {
	fulciolog.Info("default", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
