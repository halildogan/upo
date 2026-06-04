# CRD Reference

All resources are in API group **`platform.upo.io`**, version **`v1alpha1`**.

| Kind | Scope | Short | Purpose |
|------|-------|-------|---------|
| `Tenant` | Cluster | `tn` | Isolated namespace + quota + limits + NetworkPolicy + admin RBAC |
| `Environment` | Namespaced | `env` | Tenant-scoped workload namespace, config projection, GitOps delivery, TTL |
| `Integration` | Namespaced | `intg` | External connector with managed credentials, health probing, retry |

---

## Tenant

Cluster-scoped. The unit of multi-tenant isolation.

### Spec

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `displayName` | string | — | Human-friendly label. |
| `tier` | enum `free\|standard\|enterprise` | `standard` | Operational tier. |
| `namespacePrefix` | string | `tenant` | Prefix for the provisioned namespace (`<prefix>-<name>`). **Immutable.** |
| `quota.hard` | ResourceList | — | Hard limits mapped onto a `ResourceQuota`. |
| `limits.{default,defaultRequest,max}` | ResourceList | — | Per-container `LimitRange` policy. |
| `networkIsolation` | enum `None\|Baseline\|Strict` | `Baseline` | Default `NetworkPolicy` posture. |
| `admins[]` | SubjectReference | — | Subjects bound to the `admin` ClusterRole in-namespace. |
| `extraLabels` | map | — | Labels propagated to the namespace and managed children. |
| `lifecycle.suspended` | bool | `false` | Cordon workloads (pods quota → 0) without data loss. |
| `lifecycle.deletionPolicy` | enum `Delete\|Orphan` | `Delete` | Namespace fate on tenant deletion. |

### Status

`phase` (`Pending\|Provisioning\|Active\|Suspended\|Terminating\|Failed`),
`conditions[]` (`Ready`, `Progressing`, `Degraded`), `namespace`,
`environmentCount`, `integrationCount`, `observedGeneration`, `lastReconcileTime`.

---

## Environment

Namespaced (lives in the tenant namespace). Provisions a separate workload
namespace and, optionally, drives a GitOps engine.

### Spec

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tenantRef.name` | string | — | Owning Tenant. **Required, immutable.** |
| `type` | enum `development\|staging\|production\|preview\|ephemeral` | — | **Required, immutable.** |
| `profile` | enum `small\|medium\|large\|xlarge` | `medium` | Compute footprint of the workload namespace. |
| `source.{repoURL,revision,path}` | object | `revision=HEAD,path=.` | Git source (required when `gitops.provider != none`). |
| `gitops.provider` | enum `none\|argocd\|flux` | `none` | GitOps delivery engine. |
| `gitops.autoSync` / `gitops.prune` | bool | `false` | Automated sync / prune behaviour. |
| `ttl` | duration | — | Lifetime for `ephemeral`; **required** for that type. |
| `domain` | string | — | Ingress host surfaced in `status.url`. |
| `variables` | map | — | Non-secret config projected into a `ConfigMap`. |
| `secretRefs[]` | SecretKeySelector | — | Secret keys replicated into the workload namespace. |
| `suspend` | bool | `false` | Pause reconciliation/sync. |

### Status

`phase` (`Pending\|Provisioning\|Syncing\|Healthy\|Degraded\|Suspended\|Terminating\|Failed`),
`conditions[]`, `targetNamespace`, `sync.{revision,phase,lastSyncTime,message}`,
`url`, `expiresAt`, `observedGeneration`, `lastReconcileTime`.

---

## Integration

Namespaced. Models an external connector as a declarative resource.

### Spec

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tenantRef.name` | string | — | Owning Tenant. **Required, immutable.** |
| `type` | enum `webhook\|restapi\|oauth2\|database\|messaging\|objectstore` | — | **Required, immutable.** |
| `endpoint` | string | — | Base URL or DSN. **Required.** |
| `authSecretRef.name` | string | — | Credentials Secret (same namespace). |
| `headers` | map | — | Static outbound headers. |
| `events[]` | string set | — | Subscribed platform event types. |
| `retryPolicy.{maxRetries,backoff,initialDelay,maxDelay}` | object | `5,exponential` | Transient-failure retry. |
| `rateLimit.{requestsPerSecond,burst}` | object | — | Client-side throttling. |
| `healthCheck.{path,intervalSeconds,timeoutSeconds}` | object | `/healthz,60,10` | Active probing. |
| `tls.{insecureSkipVerify,caSecretRef}` | object | — | Transport security. |
| `suspend` | bool | `false` | Disable without deletion. |

### Status

`phase` (`Pending\|Connecting\|Connected\|Degraded\|Failed\|Disabled`),
`conditions[]` (incl. `SecretResolved`, `Connected`), `connectionSecretName`,
`observedEndpoint`, `retries`, `deliveredEvents`, `failedEvents`,
`lastEventTime`, `lastProbeTime`, `lastError`, `observedGeneration`,
`lastReconcileTime`.

---

## Conditions

Every resource reports the same three positive-polarity conditions, following
Kubernetes API conventions so they interoperate with `kubectl wait`:

- **`Ready`** — observed state fully matches desired spec at `observedGeneration`.
- **`Progressing`** — controller is actively converging.
- **`Degraded`** — reconciliation is blocked/failing and needs attention.
```
