package utils

import (
	"slices"
	"strings"

	v1 "k8s.io/api/core/v1"
)

// GetOrDefault retrieves the value from the map if present,
// otherwise returns the specified default value.
func GetOrDefault(m map[string]string, key string, defaultValue string) string {
	if val, exists := m[key]; exists {
		return val
	}
	return defaultValue
}

// MergeImagePullSecrets merges two lists of ImagePullSecrets
func MergeImagePullSecrets(base, override []v1.LocalObjectReference) []v1.LocalObjectReference {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}

	secrets := make(map[string]v1.LocalObjectReference)

	addSecrets := func(list []v1.LocalObjectReference) {
		for _, secret := range list {
			if secret.Name != "" {
				secrets[secret.Name] = secret
			}
		}
	}

	addSecrets(base)
	addSecrets(override)

	result := make([]v1.LocalObjectReference, 0, len(secrets))
	for _, secret := range secrets {
		result = append(result, secret)
	}
	slices.SortFunc(result, func(lhs, rhs v1.LocalObjectReference) int {
		return strings.Compare(lhs.Name, rhs.Name)
	})

	return result
}
