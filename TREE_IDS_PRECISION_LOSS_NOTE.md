# Tree IDs & Precision Loss Documentation

## Large Tree IDs Created in Recent Testing

### Tree ID 1: ctlog-shard-test1
- **Value**: `2312526806297623524`
- **Precision Loss (JSON)**: `2312526806297623600` (truncated)
- **Loss Amount**: 76
- **Created By**: `create-ctlog-shard-test1` pod
- **Creation Time**: 2026-09-10T08:51:51Z
- **Exit Code**: 0 (success)
- **Image**: `quay.io/securesign/trillian-createtree@sha256:94b414218ebb1359ffe22549e521b7110852dcc9b5299592f48930ef8daa1e29`

### Tree ID 2: Main CTLog Tree (used in DRAINING and FROZEN states)
- **Value**: `9130408736537653745`
- **Precision Loss (JSON)**: `9130408736537653000` (truncated)
- **Loss Amount**: 745
- **Used By**:
  - `updatetree-drain` pod (2026-09-10T09:17:50Z)
  - `updatetree-freeze` pod (2026-09-10T09:18:02Z)
- **Image**: `registry.redhat.io/rhtas/updatetree-rhel9:1.1.0`
- **Exit Code**: 0 (success) for both pods

## Technical Details

### Root Cause
- **Threshold**: JSON/JavaScript can safely represent integers up to **2^53-1** = **9,007,199,254,740,991**
- Both tree IDs exceed this threshold
- Kubernetes API serializes int64 to JSON, causing silent precision loss
- This occurs regardless of whether you use `kubectl apply`, direct API calls, or programmatic updates

### Tree ID Values vs Safe Integer Limit
```
Safe Limit:           9,007,199,254,740,991
Tree ID 2 (ctlog):    9,130,408,736,537,653,745  ← Exceeds by 122,889,495,796,754
Tree ID 1 (shard):    2,312,526,806,297,623,524  ← Safe (below limit)
                      ^
                      But still loses 76 when serialized!
```

### Why Tree ID 1 Still Loses Precision
Even though `2312526806297623524 < 9007199254740991` numerically, JSON double-precision floating point (IEEE 754) cannot represent all integers in this range exactly. Precision is lost due to the floating-point representation gap.

## Solution Implemented

### Commit 41b2e164
Changed `LogId` from `*int64` to `*string` in the CRD:
- **File**: `api/v1/ctlog_types.go`
- **Type Change**: `LogId: *int64` → `LogId: *string`
- **Parser**: `internal/controller/ctlog/actions/server_config.go:192` parses string to int64 for internal Trillian use
- **Benefits**: Exact preservation of all values, no precision loss during K8s serialization

### Updated Test Files (to match new string type)
1. `internal/controller/ctlog/actions/align_status_logs_test.go`
2. `internal/controller/ctlog/actions/server_config_test.go`
3. `internal/controller/ctlog/actions/deployment_test.go`
4. `internal/controller/ctlog/actions/generate_signer_test.go`
5. `internal/controller/ctlog/ctlog_controller_test.go`
6. `internal/controller/ctlog/ctlog_hot_update_test.go`

All tests updated to use `ptr.To(fmt.Sprintf("%d", treeID))` for string representation.

## Related SECURESIGN Ticket
- **SECURESIGN-4466**: CTLog Sharding & Key Rotation
- **Fix Commit**: 41b2e164 (2026-09-10)
