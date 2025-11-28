package v1alpha1

import (
	"testing"

	"k8s.io/utils/ptr"
)

func TestMonitoringConfig_IsServiceMonitorEnabled(t *testing.T) {
	tests := []struct {
		name           string
		config         MonitoringConfig
		isOpenShift    bool
		expectedResult bool
	}{
		{
			name: "ServiceMonitor explicitly set to true",
			config: MonitoringConfig{
				Enabled:        true,
				ServiceMonitor: ptr.To(true),
			},
			isOpenShift:    false,
			expectedResult: true,
		},
		{
			name: "ServiceMonitor explicitly set to false",
			config: MonitoringConfig{
				Enabled:        true,
				ServiceMonitor: ptr.To(false),
			},
			isOpenShift:    true,
			expectedResult: false,
		},
		{
			name: "ServiceMonitor nil on OpenShift defaults to true",
			config: MonitoringConfig{
				Enabled:        true,
				ServiceMonitor: nil,
			},
			isOpenShift:    true,
			expectedResult: true,
		},
		{
			name: "ServiceMonitor nil on non-OpenShift defaults to false",
			config: MonitoringConfig{
				Enabled:        true,
				ServiceMonitor: nil,
			},
			isOpenShift:    false,
			expectedResult: false,
		},
		{
			name: "ServiceMonitor explicitly true overrides platform on non-OpenShift",
			config: MonitoringConfig{
				Enabled:        true,
				ServiceMonitor: ptr.To(true),
			},
			isOpenShift:    false,
			expectedResult: true,
		},
		{
			name: "ServiceMonitor explicitly false overrides platform on OpenShift",
			config: MonitoringConfig{
				Enabled:        true,
				ServiceMonitor: ptr.To(false),
			},
			isOpenShift:    true,
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsServiceMonitorEnabled(tt.isOpenShift)
			if result != tt.expectedResult {
				t.Errorf("IsServiceMonitorEnabled() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}
