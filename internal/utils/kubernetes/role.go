package kubernetes

import (
	rbacv1 "k8s.io/api/rbac/v1"
)

func EnsureClusterRoleRules(rules ...rbacv1.PolicyRule) func(*rbacv1.ClusterRole) error {
	return func(instance *rbacv1.ClusterRole) error {
		instance.Rules = rules
		return nil
	}
}
