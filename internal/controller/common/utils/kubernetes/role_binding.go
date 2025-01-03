package kubernetes

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateRoleBinding(namespace string, name string, labels map[string]string, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		RoleRef:  roleRef,
		Subjects: subjects,
	}
}

func CreateClusterRoleBinding(name string, labels map[string]string, roleRef rbacv1.RoleRef, subjects []rbacv1.Subject) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		RoleRef:  roleRef,
		Subjects: subjects,
	}
}

func EnsureRoleBinding(role rbacv1.RoleRef, subject ...rbacv1.Subject) func(*rbacv1.RoleBinding) error {
	return func(instance *rbacv1.RoleBinding) error {
		instance.RoleRef = role
		instance.Subjects = subject
		return nil
	}
}
