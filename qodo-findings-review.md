# CTLog Sharding PR — Qodo findings, reproduced

All six Qodo findings are real and reproduce. I added failing tests that demonstrate each one and changed **no production code**. Two additional issues turned up while verifying them.

**Test files** (every test is prefixed `TestQodo_`):
- `api/v1alpha1/conversion_qodo_repro_test.go`
- `internal/controller/ctlog/utils/ctlog_config_qodo_repro_test.go`
- `internal/controller/ctlog/actions/qodo_repro_test.go`

Each assertion message names the code responsible, so they double as the spec for the fix.

---

## Findings beyond Qodo's list

**`CTLogConfig.Mirror` is dead API surface.** The field is declared in the v1 API and read by nothing in `internal/`. Removing it after GA is a breaking change — it should be deleted or wired up before v1 is finalized.

**The password-ref migration path described as "survives in status" is not implemented.** See finding #3 below — this one contradicts a stated design assumption, so it's worth reading in full.

---

## Findings

### 1. Conversion discards v1alpha1 edits — `ConvertTo` (CTlog and Securesign)

`CTlog.ConvertTo` does `dst.Spec.Logs = restored.Spec.Logs` and `dst.Status.Logs = restored.Status.Logs`, overwriting the per-log merge that `Convert_v1alpha1_CTlogSpec_To_v1_CTlogSpec` performed moments earlier. `Securesign.ConvertTo` has the same defect via `dst.Spec.Ctlog.Logs`.

Any v1alpha1 edit to `treeID`, `privateKeyRef`, or `rootCertificates` is silently ignored. This hits the ordinary `kubectl get -o yaml` → edit → `apply` workflow for any CR that has ever been served as v1alpha1 — i.e. the entire upgrade cohort. A CR with no conversion annotation is unaffected, because `ConvertTo` returns early before the overwrite.

The fix is a per-log merge into the `trusted-artifact-signer` entry instead of a wholesale slice restore. v1-only shards are already preserved correctly and must stay that way.

> `TestQodo_CTlogConvertTo_DiscardsLegacySpecEdits`, `TestQodo_CTlogConvertTo_DiscardsLegacyStatusEdits`, `TestQodo_SecuresignConvertTo_DiscardsLegacyCtlogEdits`

### 2. `resolveAllLogs` panics on the minimal valid v1 spec

`serverConfig.resolveAllLogs` reads `specLog.Signer.Type` with no nil check. `Signer` is optional in `CTLogConfig` — the CRD only requires it for non-active logs — and `generate_signer`'s `IsEnabled` explicitly handles `Signer == nil` by generating file keys. So this spec is valid, reaches config generation, and panics:

```yaml
logs:
  - prefix: trusted-artifact-signer
    active: true
```

Suggested fix, matching what `generate_signer` already assumes: treat nil/empty signer as file mode, error on anything unrecognised. Signerless *mirror* semantics can be defined later — that's additive.

`RecoverPanic` is enabled in `cmd/main.go`, so the blast radius is one CR stuck in a requeue loop with repeating stack traces, not an operator outage.

A second unguarded deref sits one line above: `GetLog` returns nil when `status.logs` holds a prefix absent from `spec.logs`, and `specLog.Readonly` then panics. Not reachable through the action pipeline — `alignStatusLogs` runs before `serverConfig` and rebuilds status from spec — so this one is defensive hardening only.

> `TestQodo_ResolveAllLogs_NilSignerPanics`, `TestQodo_ResolveAllLogs_MissingSpecLogPanics`

### 3. Encrypted-PEM password support is gone end to end

`ShardConfig.PrivateKeyPassword` still exists and `keys.go` still handles encrypted PEM blocks, but nothing populates the field and `marshalLogConfig` never sets `keyspb.PEMKeyFile.Password`. CTFE receives a `PEMKeyFile` with an empty password and cannot open the key.

**The design assumption that old instances keep the value in status does not hold.** I traced `ConvertTo` on a v1alpha1 instance with both refs set:

```
V1OBJ       (no password ref anywhere in spec or status)
ANNOT       map[]
BACKSPEC    <nil>
BACKSTATUS  <nil>
```

`CTlogLogStatus` and `CTlogStatus` have no password field, `ctlog_conversion.go` never references `PrivateKeyPassword`, and there is no `migration.Set` stash for ctlog. Fulcio implements exactly this pattern — `api/v1/fulcio_types.go` retains `PrivateKeyPasswordRef` in v1 status "for backward compatibility with the deprecated spec field" — but CTLog did not get it.

The rejection half isn't enforced either: `privateKeyPasswordRef` is still in the **served** v1alpha1 CRD schema (spec and status), its CEL rule permits it, and the only ctlog webhook is a mutating one scoped to v1. New v1alpha1 instances that set it are accepted and silently discarded, not rejected.

Consequence: for an existing instance with a BYO encrypted key, the *reference* is destroyed on the first write as v1. The secret survives in the cluster, but nothing records which secret and key held the password, so a later release has nothing to migrate from.

> `TestQodo_ResolveAllLogs_NeverResolvesKeyPassword`, `TestQodo_CreateConfig_DropsEncryptedPEMPassword`

### 4. Conflicting PKCS#11 module paths silently ignored

`ensureDeployment` breaks out of the loop after the first PKCS#11 log, so a second shard declaring a different `modulePath` never has its module loaded. CTFE accepts one process-wide `--pkcs11_module_path`, and `keyspb.PKCS11Config` carries no per-log override. The mismatch produces no error.

Simplest resolution that requires no API change: keep `modulePath` per-log and reject configurations where PKCS#11 logs declare differing paths.

> `TestQodo_Deployment_IgnoresSecondPKCS11ModulePath`

### 5. FIPS validation skips the auto-discovered Fulcio root

`ctlogCryptoMaterial` walks only `spec.logs[*].rootCerts`. When the active log's root is auto-discovered, `handleFulcioCert` writes it to `status.logs[*].rootCertificates`, and that is the certificate `server_config` resolves and mounts. The generic FIPS action then marks the resource valid having validated nothing. Low practical risk when Fulcio is operator-managed; matters for BYO-Fulcio.

> `TestQodo_FIPSMaterial_SkipsAutodiscoveredRootCert`

---

## Suggested release split

Given that v1 API finalization is the gate, the dividing line is *reversibility*: relaxing validation and adding fields later is non-breaking, removing or tightening is not.

**Must land before v1 is frozen**

| Item | Reason |
|---|---|
| `mirror` field — delete or implement | Removing a field post-GA is breaking |
| Nil-signer guard (#2) | Simplest valid spec crash-loops |
| Password-ref decision (#3) | Only irreversible data-loss item |
| Conversion merge (#1) | Silently ignores edits on the headline upgrade path; no API impact, but cheap and high-impact |

**Deferrable to a patch**

PKCS#11 module-path validation (#4) — provided `modulePath` stays per-log, so no field move is needed later. FIPS status root certs (#5). The stale-`specLog` guard.

**On #3 specifically**, this is a business call rather than a technical one. If no field deployments use a BYO encrypted CTLog PEM key, accepting the loss and documenting it as a breaking change is a legitimate and much cheaper path. If that can't be ruled out, adding `PrivateKeyPasswordRef` to `CTlogLogStatus` plus the conversion carry-over is roughly 20 lines and has to happen now — a later release has nothing to recover from. Either way the current state is the worst option, because both ends fail silently.
