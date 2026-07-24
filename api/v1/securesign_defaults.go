package v1

func (s *Securesign) SetDefaults() {
	// keep securesign minimal - defaulted on sub-resource level
	if s.Spec.Ctlog.Trillian.URL == "" && s.Spec.Ctlog.Trillian.Ref == nil {
		s.Spec.Ctlog.Trillian.Ref = &ServiceReferenceRef{
			Name:      s.Name,
			Namespace: s.Namespace,
		}
	}
	if s.Spec.Rekor.Trillian.URL == "" && s.Spec.Rekor.Trillian.Ref == nil {
		s.Spec.Rekor.Trillian.Ref = &ServiceReferenceRef{
			Name:      s.Name,
			Namespace: s.Namespace,
		}
	}
}
