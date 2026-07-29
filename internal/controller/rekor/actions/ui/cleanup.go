package ui

import (
	"context"
	"fmt"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/action"
	"github.com/securesign/operator/internal/controller/rekor/actions"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewCleanupAction() action.Action[*rhtasv1.Rekor] {
	return &cleanupAction{}
}

type cleanupAction struct {
	action.BaseAction
}

func (a cleanupAction) Name() string {
	return "cleanup-rekorsearchui"
}

func (a cleanupAction) CanHandle(_ context.Context, instance *rhtasv1.Rekor) bool {
	return meta.FindStatusCondition(instance.Status.Conditions, actions.UICondition) != nil
}

func (a cleanupAction) Handle(ctx context.Context, instance *rhtasv1.Rekor) *action.Result {
	namespace := instance.Namespace

	candidates := []client.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.SearchUiDeploymentName,
				Namespace: namespace,
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.SearchUiDeploymentName,
				Namespace: namespace,
			},
		},
		&v1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.SearchUiDeploymentName,
				Namespace: namespace,
			},
		},
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.RBACUIName,
				Namespace: namespace,
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      actions.RBACUIName,
				Namespace: namespace,
			},
		},
	}

	for _, candidate := range candidates {
		key := client.ObjectKeyFromObject(candidate)
		if err := a.Client.Get(ctx, key, candidate); err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return a.Error(ctx, fmt.Errorf("failed to get resource %s: %w", key.Name, err), instance)
		}
		if !metav1.IsControlledBy(candidate, instance) {
			continue
		}
		if err := a.Client.Delete(ctx, candidate); err != nil && !errors.IsNotFound(err) {
			return a.Error(ctx, fmt.Errorf("failed to delete resource %s: %w", key.Name, err), instance)
		}
		a.Logger.Info("Deleted orphaned RekorSearchUI resource", "name", key.Name)
	}

	meta.RemoveStatusCondition(&instance.Status.Conditions, actions.UICondition)

	return a.ReturnOnChange(a.PersistStatus)(ctx, instance)
}
