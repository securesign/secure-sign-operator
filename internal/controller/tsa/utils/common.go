package tsaUtils

import (
	"github.com/securesign/operator/api/v1alpha1"
)

func IsFileType(instance *v1alpha1.TimestampAuthority) bool {
	return GetSignerType(&instance.Spec.Signer) == FileType
}

func GetSignerType(signer *v1alpha1.TimestampAuthoritySigner) string {
	if signer.Kms != nil {
		return KmsType
	}
	if signer.Tink != nil {
		return TinkType
	}
	return FileType
}
