package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var fulciolog = logf.Log.WithName("fulcio-resource")

// FulcioDefaulter is a no-op scaffold; real defaulting logic will be added in SECURESIGN-4581.
type FulcioDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1alpha1-fulcio,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=fulcios,verbs=create;update,versions=v1alpha1,name=mfulcio.kb.io,admissionReviewVersions=v1

func SetupFulcioWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Fulcio{}).
		WithDefaulter(&FulcioDefaulter{}).
		Complete()
}

func (d *FulcioDefaulter) Default(_ context.Context, obj *Fulcio) error {
	fulciolog.Info("default", "name", obj.Name)
	return nil
}
