package api

import (
	"strings"

	"github.com/securesign/operator/api/v1alpha1"
	"github.com/securesign/operator/internal/controller/console/actions"
	"github.com/securesign/operator/internal/utils/kubernetes"
	"github.com/securesign/operator/internal/utils/kubernetes/ensure"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
)

func ensureDbAuth(instance *v1alpha1.Console, containerName string) []func(dp *apps.Deployment) error {
	return []func(dp *apps.Deployment) error{
		// ensure user auth
		func(deploy *apps.Deployment) error {
			ref := &deploy.Spec.Template.Spec
			err := ensure.ContainerAuth(kubernetes.FindContainerByNameOrCreate(ref, containerName), instance.Spec.Auth)(ref)
			return err
		},

		// ensure dbSecret auth
		ensure.Optional(instance.Status.Db.DatabaseSecretRef != nil,
			func(deploy *apps.Deployment) error {
				ref := &deploy.Spec.Template.Spec
				err := ensure.ContainerAuth(kubernetes.FindContainerByNameOrCreate(ref, containerName), dbSecretToAuth(instance.Status.Db.DatabaseSecretRef))(ref)
				return err
			}),
	}
}

func dbSecretToAuth(databaseSecretRef *v1alpha1.LocalObjectReference) *v1alpha1.Auth {
	auth := v1alpha1.Auth{}
	keys := []string{actions.SecretUser, actions.SecretPassword, actions.SecretHost, actions.SecretPort, actions.SecretDatabaseName, actions.SecretDsn}

	for _, v := range keys {
		temp := strings.ReplaceAll(v, "-", "_")
		temp = strings.ToUpper(temp)

		auth.Env = append(auth.Env, core.EnvVar{
			Name: temp,
			ValueFrom: &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					Key: v,
					LocalObjectReference: core.LocalObjectReference{
						Name: databaseSecretRef.Name,
					},
				},
			},
		})
	}
	return &auth
}
