package v1

func (s *Securesign) SetDefaults() {
	// keep securesign minimal - component defaults are handled by sub-resource webhooks
	// exception: irreversible fields defaulted to true must be defaulted here so
	// CEL transition rules fire at the Securesign level, not only on the child resource
	s.Spec.Rekor.Attestations.SetDefaults()
	s.Spec.Rekor.BackFillRedis.SetDefaults()

	// CTLog Logs must be defaulted here so users can modify them on the Securesign CR
	s.Spec.Ctlog.SetDefaults()

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
	if s.Spec.Fulcio.Ctlog.URL == "" && s.Spec.Fulcio.Ctlog.Ref == nil {
		s.Spec.Fulcio.Ctlog.Ref = &ServiceReferenceRef{
			Name:      s.Name,
			Namespace: s.Namespace,
		}
	}

	siblingRef := &ServiceReferenceRef{Name: s.Name, Namespace: s.Namespace}
	if len(s.Spec.Tuf.Rekor) == 0 {
		s.Spec.Tuf.Rekor = []TrustRootBinding{{ServiceReference: ServiceReference{Ref: siblingRef}}}
	}
	if len(s.Spec.Tuf.Fulcio) == 0 {
		s.Spec.Tuf.Fulcio = []TrustRootBindingWithOIDC{{TrustRootBinding: TrustRootBinding{ServiceReference: ServiceReference{Ref: siblingRef}}}}
	}
	if len(s.Spec.Tuf.Ctlog) == 0 {
		s.Spec.Tuf.Ctlog = []TrustRootBinding{{ServiceReference: ServiceReference{Ref: siblingRef}}}
	}
	// Tsa is a tri-state pointer, with nil meaning TSA is excluded from the trust root
	// entirely. Only populate a sibling ref when TimestampAuthority is actually
	// configured, and only if the user hasn't already set an explicit override.
	if s.Spec.TimestampAuthority != nil && (s.Spec.Tuf.Tsa == nil || len(*s.Spec.Tuf.Tsa) == 0) {
		s.Spec.Tuf.Tsa = &[]TrustRootBinding{{ServiceReference: ServiceReference{Ref: siblingRef}}}
	}
}
