package v1

import (
	"time"

	k8sresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func (s *MonitoringConfig) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(true))
}

func (s *MonitoringWithTLogConfig) SetDefaults() {
	s.MonitoringConfig.SetDefaults()
	s.TLog.SetDefaults()
}

func (s *ExternalAccess) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(false))
}

func (s *TlogMonitoring) SetDefaults() {
	setDefault(&s.Enabled, ptr.To(false))
	setDefault(&s.Interval, metav1.Duration{Duration: 10 * time.Minute})
}

func (s *TrillianService) SetDefaults() {
	setDefault(&s.Port, ptr.To(int32(8091)))
}

func (s *CtlogService) SetDefaults() {
	setDefault(&s.Prefix, "trusted-artifact-signer")
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
