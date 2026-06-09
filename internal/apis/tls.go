package apis

import rhtasv1 "github.com/securesign/operator/api/v1"

type TlsClient interface {
	GetTrustedCA() *rhtasv1.LocalObjectReference
}
