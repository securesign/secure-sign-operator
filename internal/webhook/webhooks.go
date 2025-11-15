package webhooks

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	admission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SecureSignValidator checks for namespace security policy compliance.
type SecureSignValidator struct {
	Client client.Client
}

var _ admission.CustomValidator = &SecureSignValidator{}

// Reserved OpenShift run-level labels to block
var reservedRunLevels = map[string]bool{
	"0": true, // Critical infrastructure
	"1": true, // Infrastructure
	"9": true, // General platform services
}
