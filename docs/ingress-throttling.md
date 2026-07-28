# Ingress Throttling

On OpenShift, the Operator applies HAProxy rate-limiting and connection-limit
annotations to the Route generated for each externally-exposed component's
Ingress. These client-facing endpoints are natural targets for abusive or
accidental traffic spikes, so throttling is enabled by default whenever
external access is enabled.

Supported components: **Fulcio**, **Rekor** (server), **TSA**, and **RHTAS
Console**.

## Default behavior

When `spec.ingress.enabled` is `true` and the cluster is OpenShift, the
Operator sets the following defaults on the component's Route unless
overridden:

| Component | Concurrent TCP connections per source IP | HTTP requests/sec per source IP | TCP connections/sec per source IP | Rationale |
|---|---|---|---|---|
| Fulcio | 100 | 50 | 100 | Signing endpoint, moderate traffic |
| Rekor | 200 | 100 | 200 | Every sign and verify operation hits Rekor, and TUF clients can fetch multiple files per refresh |
| TSA | 100 | 50 | 100 | Optional timestamp endpoint, similar traffic to Fulcio |
| Console | 100 | 50 | 100 | UI dashboard, same baseline |

These map to the following HAProxy annotations:

| Setting | HAProxy annotation |
|---|---|
| Enables rate-limiting on the Route | `haproxy.router.openshift.io/rate-limit-connections` (`"true"`) |
| Concurrent TCP connections per source IP | `haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp` |
| HTTP requests per second per source IP | `haproxy.router.openshift.io/rate-limit-connections.rate-http` |
| TCP connections per second per source IP | `haproxy.router.openshift.io/rate-limit-connections.rate-tcp` |

No configuration is required to get these defaults — omit `spec.throttling`
entirely.

## Customizing throttling values

Set any subset of `spec.throttling` fields on the component CR (`Fulcio`,
`Rekor`, `TimestampAuthority`) — or `spec.ui.throttling` on `Console` — or the
equivalent path on `Securesign`, to override individual defaults. Unset fields
keep their default value.

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Rekor
metadata:
  name: example-instance
spec:
  ingress:
    enabled: true
  throttling:
    concurrentTCP: 400
    rateHTTP: 200
    rateTCP: 400
  # other specifications
```

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Console
metadata:
  name: example-instance
spec:
  ui:
    ingress:
      enabled: true
    throttling:
      concurrentTCP: 200
  # other specifications
```

## Disabling throttling

Set `throttling.enabled` to `false` to remove throttling entirely. The
Operator actively removes the HAProxy annotations from the Route if they were
previously set.

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Fulcio
metadata:
  name: example-instance
spec:
  ingress:
    enabled: true
  throttling:
    enabled: false
  # other specifications
```

## Non-OpenShift clusters

`throttling` has no effect on plain Kubernetes Ingress — the HAProxy
annotations are OpenShift-Route-specific and are never applied outside
OpenShift, regardless of the CR configuration.
