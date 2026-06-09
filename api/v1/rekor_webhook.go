package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var rekorlog = logf.Log.WithName("rekor-resource")

// RekorDefaulter is a no-op scaffold; real defaulting logic will be added in SECURESIGN-4581.
type RekorDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-rekor,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=rekors,verbs=create;update,versions=v1,name=mrekor.kb.io,admissionReviewVersions=v1

func SetupRekorWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Rekor{}).
		WithDefaulter(&RekorDefaulter{}).
		Complete()
}

func (d *RekorDefaulter) Default(_ context.Context, obj *Rekor) error {
	rekorlog.Info("default", "name", obj.Name)
	return nil
}
