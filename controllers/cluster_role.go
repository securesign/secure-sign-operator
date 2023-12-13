package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	rbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensureClusterRole(ctx context.Context, securesign *rhtasv1alpha1.Securesign, roleName string) (*rbac.ClusterRole, error) {
	log := ctrllog.FromContext(ctx)

	role := &rbac.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: roleName,
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "create"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"deployments"},
				Verbs:     []string{"get", "list"},
			},
		},
	}

	err := r.Get(ctx, client.ObjectKey{Name: roleName}, role)
	if err != nil {
		log.Info("Creating ClusterRole", "ClusterRole.Namespace", role.Namespace, "ClusterRole.Name", role.Name)
		err = r.Create(ctx, role)
		if err != nil {
			log.Error(err, "Failed to create new ClusterRole", "ClusterRole.Namespace", role.Namespace, "ClusterRole.Name", role.Name)
			return nil, err
		}
	}
	return role, nil
}
