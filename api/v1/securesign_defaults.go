package v1

func (s *SecuresignSpec) SetDefaults() {
	s.Rekor.SetDefaults()
	s.Fulcio.SetDefaults()
	s.Trillian.SetDefaults()
	s.Tuf.SetDefaults()
	s.Ctlog.SetDefaults()
	if s.TimestampAuthority != nil {
		s.TimestampAuthority.SetDefaults()
	}
}
