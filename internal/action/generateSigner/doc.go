// Package generateSigner provides a generic action for managing operator-managed
// signer secrets (private keys, certificates, credential bundles).
//
// # Workflow
//
// The action follows a create-once, verify-always pattern. Signer keys are
// generated ONLY on fresh installation. After creation the secret is immutable
// (Kubernetes enforces data immutability). Most errors are retriable —
// terminal errors come from component-level [Config.ResolveRef] callbacks
// (e.g., invalid spec combinations such as a missing private key or CA
// certificate) and from [Config.GenerateData] callbacks.
//
// Handle executes the following decision tree on every reconcile:
//
//  1. Call ResolveRef. If it returns a non-nil [SecretKeySelector], the signer
//     was resolved from existing state (user-provided refs or pre-existing
//     secrets from previous operator versions). Apply TUF autodiscovery labels
//     to the resolved secret, call AlignStatus, and mark condition True.
//
//  2. Look up the operator-managed secret by its deterministic name
//     (fmt.Sprintf(format, instance.Name)). If found, call AlignStatus and
//     mark condition True.
//
//  3. If the deterministic name is not found, this is a fresh installation.
//     Call GenerateData to produce key material, create an immutable secret
//     with the deterministic name, call AlignStatus, and mark condition True.
//
// # Config Callbacks
//
// Each component provides a thin wrapper with these callbacks:
//
//   - ResolveRef: return a [SecretKeySelector] pointing to an existing secret
//     (user-provided or upgrade path). For cert-based components, return the
//     cert/chain ref (not the private key ref) so TUF autodiscovery labels
//     target the correct secret. Return nil to fall through to generation.
//   - GenerateData: produce key/cert material for fresh installation.
//   - AlignStatus: write secret reference fields into component-specific status.
//     Receives a [SecretKeySelector] with the secret name. Components check
//     instance.Spec to decide whether to copy user-provided refs or construct
//     refs with well-known keys.
//   - IsEnabled: (optional) return false for paths that don't need a secret (KMS, Tink).
//   - MutateSecret: (optional) modify the secret before creation — typically
//     adds TUF autodiscovery labels (e.g., fulcio_v1.crt.pem, tsa.certchain.pem).
//     Also applied to user-provided secrets in the resolved path as a temporary
//     workaround until dedicated resolve_pub_key actions are implemented.
//
// # Upgrade Path
//
// Secrets created by operator versions before 1.5.0 used GenerateName-based naming.
// Each component's ResolveRef callback checks the status reference via
// [ResolveStatusSecret] and, if the old secret still exists under its original
// name, returns that ref for reuse without regeneration.
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
//	            ResolveRef:   resolveRef,
//	            GenerateData: generateData,
//	            AlignStatus:  alignStatus,
//	            IsEnabled:    isEnabled,
//	        }),
//	    )
//	}
package generateSigner
