// Package fips provides a generic action for validating cryptographic material
// and settings against FIPS 140 compliance requirements.
//
// # Workflow
//
// The action runs before the signer action. When FIPS mode is disabled
// (CanHandle returns false), the action is completely skipped.
//
// When FIPS mode is active, Handle performs two checks:
//
//  1. Password-ref guard: if PasswordRef() returns a non-nil selector,
//     password-protected keys are forbidden in FIPS mode. Returns a TerminalError.
//
//  2. Crypto material validation: calls CryptoMaterial() to fetch user-provided
//     secrets, then validates each [CryptoRef] using its Validate function.
//     Returns a TerminalError for validation failures and Continue() for
//     transient secret-read failures.
//
// On success, the action sets the FIPSCondition to True and persists the
// status update.
//
// # Config Callbacks
//
// Each component provides a thin Config with these callbacks:
//
//   - PasswordRef: extract the password-ref selector from the instance.
//     Returns nil when there is no password ref (no FIPS check needed).
//   - CryptoMaterial: fetch and return user-provided crypto material for
//     FIPS validation. Returns nil when there is no user-provided material.
//   - IsEnabled: (optional) return false for paths that don't need validation
//     (e.g., KMS, Tink). Nil defaults to true.
//
// # Usage
//
//	func NewFIPSValidationAction() action.Action[*rhtasv1.Rekor] {
//	    return fips.NewAction(
//	        actions.SignerCondition,
//	        actions.ServerComponentName,
//	        fips.Wrapper(fips.Config[*rhtasv1.Rekor]{
//	            PasswordRef: func(i *rhtasv1.Rekor) *rhtasv1.SecretKeySelector {
//	                if i.Spec.Signer.KeyRef != nil {
//	                    return i.Spec.Signer.PasswordRef
//	                }
//	                return nil
//	            },
//	            CryptoMaterial: rekorCryptoMaterial,
//	        }),
//	    )
//	}
package fips
