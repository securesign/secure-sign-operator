package tsaUtils

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
)

func IsFileType(instance *rhtasv1.TimestampAuthority) bool {
	return instance.Spec.Signer.Type == "" || instance.Spec.Signer.Type == rhtasv1.SignerTypeFile
}
