package v1

import "k8s.io/utils/ptr"

func (s *TrillianSpec) SetDefaults() {
	s.Db.SetDefaults()
	s.Monitoring.SetDefaults()
	s.LogServer.SetDefaults()
	s.LogSigner.SetDefaults()
	setDefault(&s.MaxRecvMessageSize, ptr.To(int64(153600)))
}

func (s *TrillianDB) SetDefaults() {
	setDefault(&s.Create, ptr.To(true))
	s.Pvc.SetDefaults()
	setDefault(&s.Provider, "mysql")
	setDefault(&s.Uri, "$(MYSQL_USER):$(MYSQL_PASSWORD)@tcp($(MYSQL_HOST):$(MYSQL_PORT))/$(MYSQL_DATABASE)")
}
