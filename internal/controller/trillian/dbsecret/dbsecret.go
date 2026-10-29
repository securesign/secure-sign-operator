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

// EnvName maps a database secret key to its MYSQL_* environment variable name.
func EnvName(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
}

// ConnectionEnv returns the environment variables needed by database clients.
// The root password is only used by the managed database, never by clients.
func ConnectionEnv(secretName string) []core.EnvVar {
	keys := [...]string{SecretUser, SecretPassword, SecretHost, SecretPort, SecretDatabaseName}
	env := make([]core.EnvVar, 0, len(keys))
	for _, key := range keys {
		env = append(env, core.EnvVar{
			Name: EnvName(key),
			ValueFrom: &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					LocalObjectReference: core.LocalObjectReference{Name: secretName},
					Key:                  key,
				},
			},
		})
	}
	return env
}
