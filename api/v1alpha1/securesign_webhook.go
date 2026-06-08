package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var securesignlog = logf.Log.WithName("securesign-resource")

// SecuresignDefaulter is a no-op scaffold; real defaulting logic will be added in SECURESIGN-4581.
type SecuresignDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1alpha1-securesign,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=securesigns,verbs=create;update,versions=v1alpha1,name=msecuresign.kb.io,admissionReviewVersions=v1

func SetupSecuresignWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Securesign{}).
		WithDefaulter(&SecuresignDefaulter{}).
		Complete()
}

func (d *SecuresignDefaulter) Default(_ context.Context, obj *Securesign) error {
	securesignlog.Info("default", "name", obj.Name)
	return nil
}
