package v1alpha1

type Phase string

const (
	PhaseNone             Phase = ""
	PhaseInitialization   Phase = "Initialization"
	PhaseReady            Phase = "Ready"
	PhasePending          Phase = "Pending"
	PhaseError            Phase = "Error"
	PhaseDuplicitResource       = "DuplicitResource"
)
