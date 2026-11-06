package traits

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"
)

func ImagePullSecrets(secrets ...corev1.LocalObjectReference) Trait[*corev1ac.ServiceAccountApplyConfiguration] {
	return func(sa *corev1ac.ServiceAccountApplyConfiguration) error {
		for _, secret := range secrets {
			sa.WithImagePullSecrets(corev1ac.LocalObjectReference().WithName(secret.Name))
		}
		return nil
	}
}

func RoleRules(rules ...rbacv1.PolicyRule) Trait[*rbacv1ac.RoleApplyConfiguration] {
	return func(role *rbacv1ac.RoleApplyConfiguration) error {
		for _, rule := range rules {
			converted, err := ConvertViaJSON[rbacv1.PolicyRule, rbacv1ac.PolicyRuleApplyConfiguration](rule)
			if err != nil {
				return fmt.Errorf("convert role policy rule: %w", err)
			}
			role.WithRules(converted)
		}
		return nil
	}
}

func RoleBinding(role rbacv1.RoleRef, subjects ...rbacv1.Subject) Trait[*rbacv1ac.RoleBindingApplyConfiguration] {
	return func(instance *rbacv1ac.RoleBindingApplyConfiguration) error {
		instance.WithRoleRef(rbacv1ac.RoleRef().WithAPIGroup(role.APIGroup).WithKind(role.Kind).WithName(role.Name))
		for _, subject := range subjects {
			instance.WithSubjects(rbacv1ac.Subject().WithAPIGroup(subject.APIGroup).
				WithKind(subject.Kind).WithName(subject.Name).WithNamespace(subject.Namespace))
		}
		return nil
	}
}
