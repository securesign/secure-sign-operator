# Ingress Throttling

On OpenShift, the Operator applies HAProxy rate-limiting and connection-limit
annotations to the Ingress generated for each externally-exposed component,
which OpenShift propagates to the Route it auto-creates from that Ingress.
These client-facing endpoints are natural targets for abusive or accidental
traffic spikes, so throttling is enabled by default whenever external access
is enabled.

Supported components: **Fulcio**, **Rekor** (server), **TSA**, and **RHTAS
Console**.

**TUF** is not throttled: the Operator never applies HAProxy annotations to
its Ingress. OpenShift still auto-generates a Route from TUF's Ingress the
same way it does for the throttled components - it just carries no throttling
annotations. TUF's Ingress still accepts custom annotations (see
[Setting arbitrary annotations](#setting-arbitrary-annotations)).

> The legacy `spec.rekorSearchUI` field has been removed from the current
> `v1` Rekor API. On a `v1alpha1` Rekor CR, enabling it now auto-migrates it
> to a `Console` CR, which **is** throttled per the table below.

## Default behavior

When `spec.ingress.enabled` is `true` (`spec.ui.ingress.enabled` for Console)
and the cluster is OpenShift, the Operator sets the following defaults on the
component's Route unless overridden:

| Component | Concurrent TCP connections per source IP | HTTP requests/sec per source IP | TCP connections/sec per source IP | Rationale |
|---|---|---|---|---|
| Fulcio | 100 | 100 | 100 | Signing endpoint, moderate traffic |
| Rekor | 200 | 200 | 200 | Every sign and verify operation hits Rekor, and TUF clients can fetch multiple files per refresh |
| TSA | 100 | 100 | 100 | Optional timestamp endpoint, similar traffic to Fulcio |
| Console | 100 | 50 | 100 | UI dashboard, same baseline |

These map to the following HAProxy annotations, set on `spec.ingress`
(`spec.ui.ingress` for Console) and propagated by OpenShift to the
auto-generated Route:

| Setting | HAProxy annotation |
|---|---|
| Enables rate-limiting on the Route | `haproxy.router.openshift.io/rate-limit-connections` (`"true"`) |
| Concurrent TCP connections per source IP | `haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp` |
| HTTP requests per second per source IP | `haproxy.router.openshift.io/rate-limit-connections.rate-http` |
| TCP connections per second per source IP | `haproxy.router.openshift.io/rate-limit-connections.rate-tcp` |

No configuration is required to get these defaults - omit
`spec.ingress.annotations` entirely.

## Customizing throttling values

Set any subset of the HAProxy annotations above directly under
`spec.ingress.annotations` (`spec.ui.ingress.annotations` on `Console`) to
override individual defaults. Annotations you don't set keep their default
value; the Operator reapplies unset defaults on every reconcile.

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Rekor
metadata:
  name: example-instance
spec:
  ingress:
    enabled: true
    annotations:
      haproxy.router.openshift.io/rate-limit-connections.rate-http: "400"
      haproxy.router.openshift.io/rate-limit-connections.rate-tcp: "400"
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
      annotations:
        haproxy.router.openshift.io/rate-limit-connections.concurrent-tcp: "200"
  # other specifications
```

## Sizing for automated / high-throughput workloads

The defaults above are per-*source IP*, not global, and are tuned as safe
anti-abuse baselines rather than for sustained automation traffic. If many
signing clients share a single source IP as seen by the Route, for example if 
CI/CD runners behind a shared NAT gateway or egress proxy then a moderate build
fleet can exceed 100–200 requests/sec well within normal operation, since each
signing operation typically calls Fulcio, Rekor, and (if configured) TSA at
least once.

Watch for `503`/connection-reset errors from these endpoints correlating with
build concurrency, and raise `rate-http`, `rate-tcp`, and `concurrent-tcp`
under `spec.ingress.annotations` accordingly - see
[Customizing throttling values](#customizing-throttling-values). There is no
supported way to make throttling IP-aware of proxied clients (e.g. via
`X-Forwarded-For`); if your CI traffic is concentrated behind one IP, sizing
the limits up or disabling throttling for that component are the only
options.

## Disabling throttling

Set `haproxy.router.openshift.io/rate-limit-connections` to the literal,
lowercase string `"false"` under `spec.ingress.annotations` to remove
throttling entirely. The match is exact and case-sensitive - values like
`"False"` or `"0"` are treated as any other annotation value and do **not**
disable throttling. The Operator actively removes all four HAProxy annotations
above from the Route once this is set, and reapplies the defaults again if the
override is later removed.

```yaml
apiVersion: rhtas.redhat.com/v1
kind: Fulcio
metadata:
  name: example-instance
spec:
  ingress:
    enabled: true
    annotations:
      haproxy.router.openshift.io/rate-limit-connections: "false"
  # other specifications
```

## Setting arbitrary annotations

`spec.ingress.annotations` (`spec.ui.ingress.annotations` on `Console`) also
accepts any other annotation you want applied to the generated Ingress/Route -
they aren't limited to the HAProxy keys above. Non-HAProxy annotations are
applied as-is and don't interact with throttling.

## Non-OpenShift clusters

The Operator only computes and applies the HAProxy throttling defaults on
OpenShift; on plain Kubernetes it never sets or requires them. If you
explicitly set one of the HAProxy annotation keys under
`spec.ingress.annotations` anyway, the Operator still copies it onto the
Ingress like any other annotation, but it has no effect there since no
HAProxy Route processes a plain Kubernetes Ingress.
