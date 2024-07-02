package tsaUtils

import (
	"github.com/securesign/operator/api/v1alpha1"
)

const (
	FileType = "file"
	KmsType  = "kms"
)

func IsFileType(instance *v1alpha1.TimestampAuthority) bool {
	return instance.Spec.Signer.Type == FileType
}
