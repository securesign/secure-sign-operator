package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var tuflog = logf.Log.WithName("tuf-resource")

// TufDefaulter is a no-op scaffold; real defaulting logic will be added in SECURESIGN-4581.
type TufDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-tuf,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=tufs,verbs=create;update,versions=v1,name=mtuf.kb.io,admissionReviewVersions=v1

func SetupTufWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Tuf{}).
		WithDefaulter(&TufDefaulter{}).
		Complete()
}

func (d *TufDefaulter) Default(_ context.Context, obj *Tuf) error {
	tuflog.Info("default", "name", obj.Name)
	return nil
}
