package kubernetes

import (
	rbacv1 "k8s.io/api/rbac/v1"
)

func EnsureRoleBinding(role rbacv1.RoleRef, subject ...rbacv1.Subject) func(*rbacv1.RoleBinding) error {
	return func(instance *rbacv1.RoleBinding) error {
		instance.RoleRef = role
		instance.Subjects = subject
		return nil
	}
}

func EnsureClusterRoleBinding(role rbacv1.RoleRef, subject ...rbacv1.Subject) func(*rbacv1.ClusterRoleBinding) error {
	return func(instance *rbacv1.ClusterRoleBinding) error {
		instance.RoleRef = role
		instance.Subjects = subject
		return nil
	}
}
