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
