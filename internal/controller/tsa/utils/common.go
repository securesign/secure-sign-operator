package tsaUtils

import (
	rhtasv1 "github.com/securesign/operator/api/v1"
)

func IsFileType(instance *rhtasv1.TimestampAuthority) bool {
	return GetSignerType(&instance.Spec.Signer) == rhtasv1.SignerTypeFile
}

// GetSignerType returns the configured signer type, defaulting to file when unset.
// This mirrors Fulcio/CTlog/Rekor, which treat an empty type as the default backend
// (file). The type is guaranteed to be populated in practice by the defaulter and by
// the v1alpha1->v1 conversion, and CEL validation rejects a sub-struct that does not
// match the type, so empty type never coincides with a kms/tink sub-struct.
func GetSignerType(signer *rhtasv1.TimestampAuthoritySigner) string {
	if signer.Type != "" {
		return signer.Type
	}
	return rhtasv1.SignerTypeFile
}
