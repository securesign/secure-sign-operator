# TUF Key Updates for CTLog Sharding

## Overview

When CTLog's active certificate transparency log shard changes (e.g., during shard rotation), TUF must fetch the new shard's public key and publish it to the trust material so clients can verify signatures. This document explains the mechanism that ensures TUF stays synchronized with CTLog's active shard.

## Problem

**Before this fix**, TUF metadata could become stale when the active CTLog shard changed:

1. CTLog controller updates `status.Logs[]` with the new active shard (generation unchanged)
2. TUF's watch on CTLog only triggers when generation changes (via `GenerationChangedPredicate`)
3. Status-only updates don't increment generation, so watch doesn't trigger
4. TUF never reconciles, so `ResolveKeysAction` never runs
5. TUF publishes old/stale public key in metadata
6. Clients fail verification: `"ctfe public key not found for payload"`

## Root Cause

The TUF controller watched CTLog resources but only monitored generation changes:

```go
// Old: Only watches generation changes
Watches(&rhtasv1.CTlog{}, handler.EnqueueRequestsFromMapFunc(...), 
    builder.WithPredicates(crpredicate.GenerationChangedPredicate{}))
```

Generation is only incremented when `spec` changes. When CTLog controller updates `status.Logs` (the active shard), generation remains unchanged, so the watch predicate silently ignores the event.

## Solution

Added a custom predicate that specifically monitors `status.Logs` deep equality:

```go
// ctlogStatusLogsChangedPredicate triggers when CTlog's status.Logs changes
func ctlogStatusLogsChangedPredicate() crpredicate.Predicate {
    return crpredicate.Funcs{
        UpdateFunc: func(e event.UpdateEvent) bool {
            oldCTlog, ok1 := e.ObjectOld.(*rhtasv1.CTlog)
            newCTlog, ok2 := e.ObjectNew.(*rhtasv1.CTlog)
            if !ok1 || !ok2 {
                return true
            }
            return !equality.Semantic.DeepEqual(oldCTlog.Status.Logs, newCTlog.Status.Logs)
        },
    }
}
```

Applied to the TUF controller watch:

```go
Watches(&rhtasv1.CTlog{}, handler.EnqueueRequestsFromMapFunc(r.enqueueTufForCTlog), 
    builder.WithPredicates(ctlogStatusLogsChangedPredicate()))
```

## How It Works

### When Active Shard Changes

```
1. CTLog Controller                     2. TUF Controller
   Updates status.Logs                     Detects status.Logs changed
   (new active shard)                      (watch predicate triggers)
         │                                       │
         └──→ CTLog resource updated ────→ enqueueTufForCTlog()
                                                 │
                                          Enqueues TUF reconciliation
                                                 │
3. TUF Reconciliation                    4. ResolveKeysAction
   Triggered                                Calls trustroot.Resolve()
         │                                       │
         └──→ ResolveKeysAction runs ───→ Fetches active log's
                                          public key from status.Logs
                                                 │
5. TUF Publishes Key                     6. Clients Update Trust Material
   Writes ctfe.pub to                         cosign/gitsign pull new
   TUF metadata secret                        key via TUF
         │
         └──→ Clients can verify
              signatures from new shard
```

### Data Flow

```
CTLog.Status.Logs:
[
  {
    Prefix: "old-shard",      // Frozen
    Active: false,
    PublicKey: "old-key"
  },
  {
    Prefix: "new-shard",      // Active (after rotation)
    Active: true,
    PublicKey: "new-key"
  }
]
       │
       │ Watch predicate detects change
       ↓
TUF Controller Reconciles
       │
       ├─→ ResolveKeysAction
       │   │
       │   └─→ ctlogutils.ActiveLogStatus(ctlog.Status.Logs)
       │       Returns { Prefix: "new-shard", Active: true, PublicKey: "new-key" }
       │
       └─→ Publishes to TUF metadata:
           ctfe.pub = "new-key"
```

## Key Code Changes

### 1. TUF Controller (`internal/controller/tuf/tuf_controller.go`)

Added predicate to CTLog watch:

```go
Watches(&rhtasv1.CTlog{}, handler.EnqueueRequestsFromMapFunc(r.enqueueTufForCTlog), 
    builder.WithPredicates(ctlogStatusLogsChangedPredicate())).
```

Added the predicate function:

```go
func ctlogStatusLogsChangedPredicate() crpredicate.Predicate {
    return crpredicate.Funcs{
        UpdateFunc: func(e event.UpdateEvent) bool {
            oldCTlog, ok1 := e.ObjectOld.(*rhtasv1.CTlog)
            newCTlog, ok2 := e.ObjectNew.(*rhtasv1.CTlog)
            if !ok1 || !ok2 {
                return true
            }
            return !equality.Semantic.DeepEqual(oldCTlog.Status.Logs, newCTlog.Status.Logs)
        },
    }
}
```

### 2. Fulcio Controller (Similar pattern)

Fulcio also watches CTLog status changes to update its deployment with the correct active shard prefix:

```go
Watches(&rhtasv1.CTlog{}, handler.EnqueueRequestsFromMapFunc(...), 
    builder.WithPredicates(crpredicate.Or(
        crpredicate.GenerationChangedPredicate{},
        // ... other predicates ...
        ctlogStatusLogsChangedPredicate(),
    )))
```

## Testing

Updated test fixtures to populate `Status.Logs` (required for the dynamic key resolution):

```go
ctlog.Status.Logs = []rhtasv1.CTlogLogStatus{
    {
        Prefix: "test-prefix",
        Active: true,
    },
}
```

Unit tests verify:
- ✅ TUF reconciliation triggers when status changes
- ✅ ResolveKeysAction fetches correct active key
- ✅ Metadata publishes new key for clients

## Impact

| Scenario | Before | After |
|----------|--------|-------|
| **Active shard rotates** | TUF doesn't reconcile, old key published, clients fail | TUF reconciles immediately, new key published, clients succeed |
| **Spec changes** | Works (generation incremented) | Still works (uses existing GenerationChangedPredicate) |
| **Status-only changes** | Ignored (generation unchanged) | Detected and handled (status.Logs predicate) |

## Related Changes

This fix works in conjunction with:

1. **Dynamic prefix resolution in Fulcio** (`internal/controller/fulcio/actions/deployment.go`):
   - Extracts active shard prefix from `status.Logs` dynamically
   - Builds internal HTTP URL: `http://ctlog.{ns}.svc:6963/{prefix}`

2. **CTLog API conversion** (`api/v1alpha1/ctlog_conversion.go`):
   - Preserves keys during v1alpha1→v1 migration
   - Populates `status.Logs` from spec logs

## Commits

- **6ad3360d**: Dynamic prefix resolution for Fulcio
- **5b450a64**: Watch predicates for status.Logs in Fulcio & TUF
- **a4e184e1**: Test fixtures with status.Logs
