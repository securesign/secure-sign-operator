/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package dbsecret holds the well-known Trillian DB secret keys and the
// conversion from a legacy database secret reference to Auth env vars.
// It has no controller-runtime dependencies so it can be shared by the
// v1alpha1->v1 conversion webhooks, the Trillian controller and e2e tests
// without pulling in reconciliation machinery.
package dbsecret

import (
	"strings"

	rhtasv1 "github.com/securesign/operator/api/v1"
	core "k8s.io/api/core/v1"
)

const (
	SecretRootPassword = "mysql-root-password"
	SecretPassword     = "mysql-password"
	SecretDatabaseName = "mysql-database"
	SecretUser         = "mysql-user"
	SecretPort         = "mysql-port"
	SecretHost         = "mysql-host"
)

// DbSecretToAuth builds Auth env vars from a legacy database secret reference.
// Shared with the v1alpha1->v1 conversion webhooks for the deprecated DatabaseSecretRef field.
func DbSecretToAuth(databaseSecretRef *rhtasv1.LocalObjectReference) *rhtasv1.Auth {
	if databaseSecretRef == nil {
		return nil
	}
	auth := rhtasv1.Auth{}
	keys := []string{SecretUser, SecretPassword, SecretHost, SecretPort, SecretDatabaseName}

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
