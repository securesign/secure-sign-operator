package constants

const (
	AppName = "trusted-artifact-signer"

	// conditions
	Ready      = "Ready"
	Pending    = "Pending"
	Creating   = "Creating"
	Initialize = "Initialize"
	// The operator has encountered a recoverable error and is attempting to recover. This state includes a counter (RecoveryAttempts) to track the number of recovery attempts.
	Recovering = "Recovering"
	// The operator has encountered an unrecoverable error or exceeded the maximum number of recovery attempts. Reconciliation stops.
	Failure = "Failure"
	// An error has occurred, but it’s not yet classified as recoverable or irrecoverable. This state can transition to either Recovering or Failed based on error type and policy.
	Error = "Error"
)
