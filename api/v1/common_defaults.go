package v1

import (
	"time"

	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func (s *MonitoringConfig) SetDefaults() {
	setDefault(&s.Metrics.Enabled, ptr.To(true))
	setDefault(&s.ServiceMonitor.Enabled, ptr.To(false))
}

func (s *MonitoringWithTLogConfig) SetDefaults() {
	s.MonitoringConfig.SetDefaults()
	s.TLog.SetDefaults()
}

func (s *Ingress) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(false))
}

func (s *TlogMonitoring) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(false))
	setDefault(&s.Interval, &metav1.Duration{Duration: 10 * time.Minute})
}

func (s *Pvc) SetDefaults() {
	if s.Size == nil {
		s.Size = ptr.To(k8sresource.MustParse("5Gi"))
	}
	setDefault(&s.Retain, ptr.To(true))
	setDefaultSlice(&s.AccessModes, []PersistentVolumeAccessMode{"ReadWriteOnce"})
}

func (s *PodRequirements) SetDefaults() {
	setDefault(&s.Replicas, ptr.To(int32(1)))
}

func (s *PodExtensions) SetDefaults() {
	for i := range s.Volumes {
		s.Volumes[i].SetDefaults()
	}
}

func (v *AdditionalVolume) SetDefaults() {
	defaultMode := ptr.To(int32(0644))
	if v.Secret != nil && v.Secret.DefaultMode == nil {
		v.Secret.DefaultMode = defaultMode
	}
	if v.ConfigMap != nil && v.ConfigMap.DefaultMode == nil {
		v.ConfigMap.DefaultMode = defaultMode
	}
	if v.Projected != nil && v.Projected.DefaultMode == nil {
		v.Projected.DefaultMode = defaultMode
	}
}
