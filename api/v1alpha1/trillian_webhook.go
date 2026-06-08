package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var trillianlog = logf.Log.WithName("trillian-resource")

// TrillianDefaulter is a no-op scaffold; real defaulting logic will be added in SECURESIGN-4581.
type TrillianDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1alpha1-trillian,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=trillians,verbs=create;update,versions=v1alpha1,name=mtrillian.kb.io,admissionReviewVersions=v1

func SetupTrillianWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Trillian{}).
		WithDefaulter(&TrillianDefaulter{}).
		Complete()
}

func (d *TrillianDefaulter) Default(_ context.Context, obj *Trillian) error {
	trillianlog.Info("default", "name", obj.Name)
	return nil
}
