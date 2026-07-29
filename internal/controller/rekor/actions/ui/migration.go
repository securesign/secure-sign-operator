package ui

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/migration"
	"github.com/securesign/operator/internal/utils"
	"github.com/securesign/operator/internal/utils/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const consoleNameFormat = "%s-console"

func NewMigrationAction() action.Action[*rhtasv1.Rekor] {
	return &migrationAction{}
}

type migrationAction struct {
	action.BaseAction
}

func (a migrationAction) Name() string {
	return "migrate-rekorsearchui"
}

func (a migrationAction) CanHandle(ctx context.Context, instance *rhtasv1.Rekor) bool {
	return migration.Has(instance, rhtasv1alpha1.MigrationSearchUIData)
}

func (a migrationAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	var searchUI rhtasv1alpha1.RekorSearchUI
	if ok, err := migration.Read(instance, rhtasv1alpha1.MigrationSearchUIData, &searchUI); err != nil {
		return a.Error(ctx, fmt.Errorf("failed to deserialize RekorSearchUI from annotation: %w", err), instance)
	} else if !ok {
		if err := a.removeMigrationAnnotation(ctx, instance); err != nil {
			return a.Error(ctx, err, instance)
		}
		return a.Continue()
	}

	if !utils.IsEnabled(searchUI.Enabled) {
		a.Logger.Info("RekorSearchUI not enabled, removing migration annotation")
		if err := a.removeMigrationAnnotation(ctx, instance); err != nil {
			return a.Error(ctx, err, instance)
		}
		return a.Continue()
	}

	consoleName := fmt.Sprintf(consoleNameFormat, instance.Name)
	console := &rhtasv1.Console{
		ObjectMeta: metav1.ObjectMeta{
			Name:      consoleName,
			Namespace: instance.Namespace,
		},
	}

	if _, err := kubernetes.CreateOrUpdate(ctx, a.Client, console,
		func(object *rhtasv1.Console) error {
			object.Spec.UI = rhtasv1.ConsoleUI{
				PodRequirements: rhtasv1.PodRequirements{
					Replicas:    searchUI.Replicas,
					Affinity:    searchUI.Affinity,
					Resources:   searchUI.Resources,
					Tolerations: searchUI.Tolerations,
				},
				Ingress: rhtasv1.Ingress{
					Enabled: searchUI.Enabled,
					Host:    searchUI.Host,
					Labels:  searchUI.RouteSelectorLabels,
				},
				Rekor: rhtasv1.ServiceReference{
					Ref: &rhtasv1.ServiceReferenceRef{
						Name:      instance.Name,
						Namespace: instance.Namespace,
					},
				},
			}
			return nil
		},
	); err != nil {
		return a.Error(ctx, fmt.Errorf("failed to create Console CR: %w", err), instance)
	}

	a.Logger.Info("Console CR created from RekorSearchUI migration", "name", consoleName)

	if err := a.removeMigrationAnnotation(ctx, instance); err != nil {
		return a.Error(ctx, err, instance)
	}

	return a.Continue()
}

func (a migrationAction) removeMigrationAnnotation(ctx context.Context, instance *rhtasv1.Rekor) error {
	before := instance.DeepCopy()
	migration.Remove(instance, rhtasv1alpha1.MigrationSearchUIData)
	return a.Client.Patch(ctx, instance, client.MergeFrom(before))
}
