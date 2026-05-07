package tsaUtils

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
)

const (
	FileType = "file"
	KmsType  = "kms"
	TinkType = "tink"
)

func IsFileType(instance *rhtasv1.TimestampAuthority) bool {
	return GetSignerType(&instance.Spec.Signer) == FileType
}

func GetSignerType(signer *rhtasv1.TimestampAuthoritySigner) string {
	if signer.Kms != nil {
		return KmsType
	}
	if signer.Tink != nil {
		return TinkType
	}
	return FileType
}
