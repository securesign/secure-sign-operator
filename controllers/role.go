package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	rbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *SecuresignReconciler) ensureRole(ctx context.Context, securesign *rhtasv1alpha1.Securesign, namespace string, roleName string, component string) (*rbac.Role, error) {
	log := ctrllog.FromContext(ctx)

	role := &rbac.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     component,
				"app.kubernetes.io/instance": "trusted-artifact-signer",
			},
		},
		Rules: []rbac.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: []string{component + "-config"},
				Verbs:         []string{"get", "update"},
			},
		},
	}

	// if roleNmae is ctlog-secret-operator add additional secret access
	if roleName == "ctlog-secret-operator" {
		role.Rules = append(role.Rules, rbac.PolicyRule{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"create", "get", "update"},
		})
	}

	// if roleNmae is tuf replace the contents of the rule with secrets and create, get, update, delete
	if roleName == "tuf" {
		role.Rules = []rbac.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"create", "get", "update"},
			},
		}
	}
	err := r.Get(ctx, client.ObjectKey{Name: roleName, Namespace: namespace}, role)
	if err != nil {
		log.Info("Creating Role", "Role.Namespace", role.Namespace, "Role.Name", role.Name)
		err = r.Create(ctx, role)
		if err != nil {
			log.Error(err, "Failed to create new Role", "Role.Namespace", role.Namespace, "Role.Name", role.Name)
			return nil, err
		}
	}
	return role, nil
}
