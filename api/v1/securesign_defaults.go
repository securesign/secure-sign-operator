package v1

func (s *Securesign) SetDefaults() {
	// keep securesign minimal - defaulted on sub-resource level

	// bind all services together if created by Securesign umbrella
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

	if s.Spec.Tuf.Rekor.URL == "" && s.Spec.Tuf.Rekor.Ref == nil {
		s.Spec.Tuf.Rekor.Ref = &ServiceReferenceRef{
			Name:      s.Name,
			Namespace: s.Namespace,
		}
	}
	if s.Spec.Tuf.Fulcio.URL == "" && s.Spec.Tuf.Fulcio.Ref == nil {
		s.Spec.Tuf.Fulcio.Ref = &ServiceReferenceRef{
			Name:      s.Name,
			Namespace: s.Namespace,
		}
	}
	if s.Spec.Tuf.Ctlog.URL == "" && s.Spec.Tuf.Ctlog.Ref == nil {
		s.Spec.Tuf.Ctlog.Ref = &ServiceReferenceRef{
			Name:      s.Name,
			Namespace: s.Namespace,
		}
	}
	if s.Spec.Tuf.Tsa.URL == "" && s.Spec.Tuf.Tsa.Ref == nil {
		s.Spec.Tuf.Tsa.Ref = &ServiceReferenceRef{
			Name:      s.Name,
			Namespace: s.Namespace,
		}
	}
}
