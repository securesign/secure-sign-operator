package v1alpha1

type Phase string

const (
	PhaseNone     Phase = ""
	PhaseCreating Phase = "Creating"

	PhaseInitialize Phase = "Initialization"
	PhaseReady      Phase = "Ready"
	PhasePending    Phase = "Pending"
	PhaseError      Phase = "Error"
)

type ExternalAccess struct {
	// If set to true, the Operator will create an Ingress or a Route resource.
	//For the plain Ingress there is no TLS configuration provided Route object uses "edge" termination by default.
	Enabled bool `json:"enabled,omitempty"`
	// Set hostname for your Ingress/Route.
	Host string `json:"host,omitempty"`
}
