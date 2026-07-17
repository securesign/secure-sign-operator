package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type ConsoleDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-console,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=consoles,verbs=create;update,versions=v1,name=mconsole.rhtas.redhat.com,admissionReviewVersions=v1,matchPolicy=Equivalent

func SetupConsoleWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Console{}).
		WithDefaulter(&ConsoleDefaulter{}).
		Complete()
}

func (d *ConsoleDefaulter) Default(ctx context.Context, obj *Console) error {
	logf.FromContext(ctx).WithName("Console").Info("setting defaults", "name", obj.Name)
	obj.Spec.SetDefaults()
	return nil
}
