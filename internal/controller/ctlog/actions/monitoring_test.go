package actions

import (
	"context"
	"testing"

	rhtasv1 "github.com/securesign/operator/api/v1"
	"github.com/securesign/operator/internal/config"
	"github.com/securesign/operator/internal/constants"
	"github.com/securesign/operator/internal/state"
	testAction "github.com/securesign/operator/internal/testing/action"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMonitoring_CanHandle(t *testing.T) {
	tests := []struct {
		name                string
		monitoringEnabled   bool
		monitoringAvailable bool
		phase               state.State
		want                bool
	}{
		{
			name:                "enabled and available",
			monitoringEnabled:   true,
			monitoringAvailable: true,
			phase:               state.Creating,
			want:                true,
		},
		{
			name:                "enabled but ServiceMonitor API unavailable",
			monitoringEnabled:   true,
			monitoringAvailable: false,
			phase:               state.Creating,
			want:                true, // CanHandle must return true so Handle() runs and produces a proper error
		},
		{
			name:                "disabled but available",
			monitoringEnabled:   false,
			monitoringAvailable: true,
			phase:               state.Creating,
			want:                false,
		},
		{
			name:                "enabled and available but pending",
			monitoringEnabled:   true,
			monitoringAvailable: true,
			phase:               state.Pending,
			want:                false,
		},
	}

	original := config.MonitoringAvailable
	defer func() { config.MonitoringAvailable = original }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.MonitoringAvailable = tt.monitoringAvailable

			c := testAction.FakeClientBuilder().Build()
			a := testAction.PrepareAction(c, NewCreateMonitorAction())

			enabled := tt.monitoringEnabled
			instance := rhtasv1.CTlog{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "instance",
					Namespace: "default",
				},
				Spec: rhtasv1.CTlogSpec{
					Monitoring: rhtasv1.MonitoringWithTLogConfig{
						MonitoringConfig: rhtasv1.MonitoringConfig{Enabled: &enabled},
					},
				},
			}
			meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
				Type:   constants.ReadyCondition,
				Reason: tt.phase.String(),
			})

			if got := a.CanHandle(context.TODO(), &instance); got != tt.want {
				t.Errorf("CanHandle() = %v, want %v", got, tt.want)
			}
		})
	}
}
