// Package generateSigner provides a generic action for managing operator-managed
// signer secrets (private keys, certificates, credential bundles).
//
// # Workflow
//
// The action follows a create-once, verify-always pattern. Signer keys are
// generated ONLY on fresh installation. After creation the secret is immutable
// — most mutations produce a [reconcile.TerminalError] requiring manual user
// intervention, because regenerating a different private key would break all
// existing signature verification.
//
// Handle executes the following decision tree on every reconcile:
//
//  1. Call Resolve. If it returns true, the signer was resolved from existing
//     state (user-provided refs, pre-existing secrets from previous operator
//     versions). Mark condition True and return.
//
//  2. Look up the operator-managed secret by its deterministic name
//     (fmt.Sprintf(format, instance.Name)). If found, verify integrity
//     by computing a SHA256 hash of the data and comparing it to the
//     stored hash annotation. If the hash doesn't match, return a
//     TerminalError — the data was modified externally. If it matches,
//     reuse the existing secret.
//
//  3. If the deterministic name is not found, this is a fresh installation.
//     Call GenerateData to produce key material and create an immutable
//     secret with the deterministic name. If the API server returns
//     AlreadyExists (concurrent reconcile or cache lag), fetch the existing
//     secret and verify its data hash. If valid, reuse it. If not, return
//     a TerminalError.
//
// # TerminalError Scenarios
//
// The following situations halt reconciliation and require user intervention:
//
//   - Secret data modified externally (restore from backup)
//   - Leftover secret with different configuration (delete or adopt it)
//   - Operator-managed secret deleted externally (restore from backup)
//
// The only retriable scenario is a missing user-referenced secret, which may
// not yet exist at the time the CR is created.
//
// # Config Callbacks
//
// Each component provides a thin wrapper with these callbacks:
//
//   - Resolve: return true if the signer can be resolved from existing state
//     (user refs, pre-existing secrets). Must sync status when returning true.
//   - GenerateData: produce key/cert material for fresh installation.
//   - AlignStatus: copy secret name/keys into component-specific status fields.
//   - IsEnabled: (optional) return false for paths that don't need a secret (KMS, Tink).
//   - MutateSecret: (optional) modify the secret before creation (add labels, annotations).
//
// # Upgrade Path
//
// Secrets created by operator versions before 1.5.0 used GenerateName-based naming.
// Each component's Resolve callback checks the status reference and, if the old
// secret still exists under its original name, reuses it without regeneration.
//
// # Usage
//
//	func NewGenerateSignerAction() action.Action[*rhtasv1.Rekor] {
//	    return generateSigner.NewAction(
//	        actions.SignerCondition,
//	        "rekor-signer-config-%s",
//	        actions.ServerComponentName,
//	        actions.ServerDeploymentName,
//	        generateSigner.Wrapper(generateSigner.Config[*rhtasv1.Rekor]{
//	            Resolve:      resolve,
//	            GenerateData: generateData,
//	            AlignStatus:  alignStatus,
//	            IsEnabled:    isEnabled,
//	        }),
//	    )
//	}
package generateSigner
