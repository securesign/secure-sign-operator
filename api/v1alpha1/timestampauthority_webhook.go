package v1alpha1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var timestampauthoritylog = logf.Log.WithName("timestampauthority-resource")

// TimestampAuthorityDefaulter is a no-op scaffold; real defaulting logic will be added in SECURESIGN-4581.
type TimestampAuthorityDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1alpha1-timestampauthority,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=timestampauthorities,verbs=create;update,versions=v1alpha1,name=mtimestampauthority.kb.io,admissionReviewVersions=v1

func SetupTimestampAuthorityWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &TimestampAuthority{}).
		WithDefaulter(&TimestampAuthorityDefaulter{}).
		Complete()
}

func (d *TimestampAuthorityDefaulter) Default(_ context.Context, obj *TimestampAuthority) error {
	timestampauthoritylog.Info("default", "name", obj.Name)
	return nil
}
