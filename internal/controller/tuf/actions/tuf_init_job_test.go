package actions

import (
	"testing"

	"github.com/securesign/operator/api/v1alpha1"
)

func TestHasTSAKey(t *testing.T) {
	tests := []struct {
		name     string
		keys     []v1alpha1.TufKey
		expected bool
	}{
		{
			name:     "no keys",
			keys:     nil,
			expected: false,
		},
		{
			name: "keys without TSA",
			keys: []v1alpha1.TufKey{
				{Name: "rekor.pub"},
				{Name: "ctfe.pub"},
				{Name: "fulcio_v1.crt.pem"},
			},
			expected: false,
		},
		{
			name: "keys with TSA",
			keys: []v1alpha1.TufKey{
				{Name: "rekor.pub"},
				{Name: "ctfe.pub"},
				{Name: "fulcio_v1.crt.pem"},
				{Name: "tsa.certchain.pem"},
			},
			expected: true,
		},
		{
			name: "only TSA key",
			keys: []v1alpha1.TufKey{
				{Name: "tsa.certchain.pem"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasTSAKey(tt.keys); got != tt.expected {
				t.Errorf("hasTSAKey() = %v, want %v", got, tt.expected)
			}
		})
	}
}
