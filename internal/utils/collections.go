package utils

// GetOrDefault retrieves the value from the map if present,
// otherwise returns the specified default value.
func GetOrDefault(m map[string]string, key string, defaultValue string) string {
	if val, exists := m[key]; exists {
		return val
	}
	return defaultValue
}
