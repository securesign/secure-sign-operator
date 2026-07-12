package v1

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var consolelog = logf.Log.WithName("console-resource")

// ConsoleDefaulter is a no-op scaffold for defaulting logic.
type ConsoleDefaulter struct{}

//+kubebuilder:webhook:path=/mutate-rhtas-redhat-com-v1-console,mutating=true,failurePolicy=fail,sideEffects=None,groups=rhtas.redhat.com,resources=consoles,verbs=create;update,versions=v1,name=mconsole.kb.io,admissionReviewVersions=v1

func SetupConsoleWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &Console{}).
		WithDefaulter(&ConsoleDefaulter{}).
		Complete()
}

func (d *ConsoleDefaulter) Default(_ context.Context, obj *Console) error {
	consolelog.Info("default", "name", obj.Name)
	return nil
}
