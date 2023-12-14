package controllers

import (
	"context"

	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rhtasv1alpha1 "github.com/securesign/operator/api/v1alpha1"
	rbac "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *SecuresignReconciler) ensureRoleBinding(ctx context.Context, securesign *rhtasv1alpha1.Securesign, namespace string, bindingName string, roleName string, serviceAccount string, component string, tufNS string, ctNS string) (*rbac.RoleBinding, error) {
	log := log.FromContext(ctx)

	roleBinding := &rbac.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     component,
				"app.kubernetes.io/instance": "trusted-artifact-signer",
			},
		},
		Subjects: []rbac.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount,
				Namespace: namespace,
			},
		},
		RoleRef: rbac.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
	}

	// If the bindingName is tuf-secret-copy-job* then change the kind of Role to clusterrole
	// The Namespace for the serviceAccount will be tuf-system
	if bindingName == "tuf-secret-copy-job-fulcio-binding" || bindingName == "tuf-secret-copy-job-binding" || bindingName == "tuf-secret-copy-job-rekor-binding" || bindingName == "tuf-secret-copy-job-ctlog-binding" {
		roleBinding.RoleRef.Kind = "ClusterRole"
		roleBinding.Subjects[0].Namespace = tufNS
	}
	if bindingName == "trusted-artifact-signer-ctlog-createctconfig" {
		roleBinding.Subjects[0].Namespace = ctNS
	}
	err := r.Get(ctx, client.ObjectKey{Name: bindingName, Namespace: namespace}, roleBinding)
	if err != nil {
		log.Info("Creating RoleBinding", "RoleBinding.Namespace", roleBinding.Namespace, "RoleBinding.Name", roleBinding.Name)
		err = r.Create(ctx, roleBinding)
		if err != nil {
			log.Error(err, "Failed to create new RoleBinding", "RoleBinding.Namespace", roleBinding.Namespace, "RoleBinding.Name", roleBinding.Name)
			return nil, err
		}
	}
	return roleBinding, nil
}
