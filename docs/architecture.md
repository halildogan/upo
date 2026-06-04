# Architecture & Control Loop

The Unified Platform Operator (UPO) is a Kubernetes control plane that turns
high-level platform intent — *tenants*, *environments*, and *integrations* —
into the concrete Kubernetes objects and external wiring that realize it. It
follows the standard controller-runtime, level-triggered reconciliation model:
**observe desired state → diff against actual state → converge → record status**,
repeated forever and on every relevant change.

---

## 1. System context

```
                              ┌───────────────────────────────────────────┐
                              │            Kubernetes API Server            │
   platform engineer          │   (etcd-backed declarative source of truth) │
        │ kubectl/GitOps       │                                             │
        ▼                      │   CRDs:  Tenant (cluster)                   │
  ┌───────────┐   apply CRs    │         Environment (namespaced)            │
  │  Git repo │ ─────────────► │         Integration (namespaced)            │
  └───────────┘                └───────────────┬─────────────────────────────┘
        ▲                          watch │ ▲ status/events
        │ reconcile (Argo/Flux)          ▼ │
        │                      ┌───────────────────────────────────────────┐
        │                      │      UPO Controller Manager (this repo)     │
        │                      │  ┌─────────────┐ ┌─────────────┐ ┌────────┐ │
        │                      │  │  Tenant     │ │ Environment │ │ Integr.│ │
        │                      │  │ Reconciler  │ │ Reconciler  │ │ Recon. │ │
        │                      │  └──────┬──────┘ └──────┬──────┘ └───┬────┘ │
        │                      │  admission webhooks · metrics · tracing     │
        │                      └─────────┼───────────────┼───────────┼───────┘
        │                                ▼               ▼           ▼
        │                       Namespaces, Quotas,  GitOps Apps   Connection
        │                       LimitRanges, Net-    (Argo CD /    Secrets,
        └───────────────────── Policies, RBAC        Flux CRs)     HTTP probes
                                                                   to external
                                                                   systems
```

The operator never holds authoritative state in memory: the API server (etcd)
is the single source of truth. Restarting the manager loses nothing — it simply
re-observes and re-converges.

---

## 2. The reconcile loop (per resource)

Every controller implements the same skeleton. The diagram below is the Tenant
reconciler; Environment and Integration differ only in the *provisioning* box.

```
   ┌──────────────────────────────────────────────────────────────────────┐
   │ Reconcile(ctx, req)                                                    │
   └──────────────────────────────────────────────────────────────────────┘
                 │
                 ▼
        Get(resource)
                 │ NotFound? ─────────────► clear metrics, return (no requeue)
                 ▼
        DeletionTimestamp set? ──── yes ──► finalize():
                 │                            • tear down external resources
                 │ no                         • EnsureNamespaceDeleted (requeue
                 ▼                              until gone)
        finalizer present? ── no ──► add finalizer, Update, requeue
                 │ yes
                 ▼
        ┌─────────────────────────────────────────────┐
        │ provision (idempotent CreateOrUpdate):        │
        │   namespace → quota → limits → netpol → rbac  │   any step errors?
        │   (Environment: ns → quota → config → secrets │ ───────────────┐
        │    → gitops; Integration: secret → conn → probe)              │
        └───────────────────────┬───────────────────────┘               ▼
                                 │ success                        mark Degraded,
                                 ▼                                emit Warning event,
        set conditions (Ready/Progressing), derive phase,         update status,
        observedGeneration, lastReconcileTime, metrics            return err  ──┐
                                 │                                              │
                                 ▼                                              │
        Status().Update()                                                      │
                                 │                                             │
                                 ▼                                             ▼
        return RequeueAfter(resync)                          work queue retries
                                                              with exponential
                                                              backoff
```

Key properties:

- **Idempotent.** Every provisioning step is a server-side `CreateOrUpdate`, so
  re-running reconcile is always safe and converges to the same result.
- **Level-triggered, not edge-triggered.** The controller reacts to *observed
  state*, not to a stream of events, so missed events cannot cause drift — the
  periodic resync and watch re-list guarantee eventual convergence.
- **Finalizer-guarded teardown.** External side effects (namespaces, GitOps
  apps, connection secrets) are removed before the finalizer is dropped, so
  there are no orphans.
- **Status is observable truth.** `status.conditions` (Ready/Progressing/
  Degraded) and `status.phase` are written every pass with `observedGeneration`,
  enabling `kubectl wait --for=condition=Ready` and GitOps health gating.

---

## 3. Event triggers

A reconcile is enqueued when:

1. The resource itself changes (`For(&Tenant{})`).
2. A resource it **owns** changes (`Owns(&Namespace{})`, `Owns(&ResourceQuota{})`,
   …) — owner references route the event back to the parent.
3. A **watched** related resource changes — the Tenant reconciler `Watches`
   Environments and Integrations and maps each back to its `spec.tenantRef`,
   keeping the tenant's child counts current.
4. The **resync** timer fires (`RequeueAfter`), bounding worst-case drift even
   with no events (Tenant 5m, Environment 3m, Integration 2m).
5. An explicit `RequeueAfter` from the previous pass (dependency back-off,
   retry/backoff, ephemeral TTL countdown).

---

## 4. Resource state machines

### Tenant
```
            ┌─────────┐  provision ok        ┌────────┐
  create ─► │ Pending │ ───────────────────► │ Active │ ◄────────┐
            └────┬────┘                       └───┬────┘          │ reconcile
                 │ provisioning                   │ spec.suspended │ ok
                 ▼                                ▼                │
          ┌──────────────┐                  ┌───────────┐         │
          │ Provisioning │                  │ Suspended │ ────────┘
          └──────┬───────┘                  └───────────┘
                 │ step error
                 ▼
            ┌────────┐        delete        ┌─────────────┐
            │ Failed │ ───────────────────► │ Terminating │ ─► (gone)
            └────────┘                       └─────────────┘
```

### Environment
```
  Pending ─► Provisioning ─► (gitops?) ─► Syncing ─► Healthy
                                  │                     ▲
                                  └── sync error ──► Degraded ┘
   suspend → Suspended      ttl lapsed (ephemeral) → deleted → Terminating
```

### Integration
```
  Pending ─► Connecting ─► Connected
                  │            ▲
            probe fail         │ probe ok
                  ▼            │
              Degraded ── retries ≤ max (backoff) ──┘
                  │ retries > max
                  ▼
               Failed            suspend → Disabled
```

---

## 5. Tenancy & ownership model

- **Tenant** is cluster-scoped and owns its provisioned namespace plus the
  namespaced `ResourceQuota`, `LimitRange`, `NetworkPolicy`, and admin
  `RoleBinding` (a cluster-scoped owner may own namespaced dependents, so
  garbage collection cascades correctly).
- **Environment** is namespaced (it lives in the tenant namespace) but
  provisions a *separate* workload namespace. Because a namespaced object cannot
  own a cluster-scoped Namespace, the workload namespace is reclaimed explicitly
  in the finalizer; its in-namespace children (quota, config, secrets) cascade
  with it.
- **Integration** is namespaced and owns its connection `Secret` in the same
  namespace (valid same-namespace ownership → GC on delete).

---

## 6. Cross-cutting concerns

| Concern        | Mechanism                                                                 |
|----------------|---------------------------------------------------------------------------|
| Observability  | controller-runtime metrics + custom `upo_*` Prometheus series; OTel spans |
| HA             | leader election (lease) → exactly one active manager; scale replicas for standby |
| Security       | non-root, read-only rootfs, dropped caps; secure metrics (authn/authz)    |
| Admission      | defaulting + validating webhooks enforce invariants the schema cannot     |
| Extensibility  | `internal/platform` provisioning primitives are reusable building blocks  |
```
