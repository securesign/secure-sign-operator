package apis

import "github.com/securesign/operator/api/common"

type TlsClient interface {
	GetTrustedCA() *common.LocalObjectReference
}
