package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type TimestampAuthorityDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-timestampauthority,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=timestampauthorities,verbs=create;update,versions=v1,name=mtimestampauthority.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupTimestampAuthorityWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &TimestampAuthority{}).
		WithDefaulter(&TimestampAuthorityDefaulter{}).
		Complete()
}

func (d *TimestampAuthorityDefaulter) Default(ctx context.Context, obj *TimestampAuthority) error {
	logf.FromContext(ctx).WithName("TimestampAuthority").Info("setting defaults", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
