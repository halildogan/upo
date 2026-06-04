<div align="center">

# Unified Platform Operator (UPO)

**A Kubernetes-native control plane for multi-tenant SaaS platforms.**

Declarative Tenants, Environments, and Integrations — provisioned, reconciled,
and torn down by controllers, the way Kubernetes manages everything else.

[![CI](https://github.com/halildogan/upo/actions/workflows/ci.yaml/badge.svg)](https://github.com/halildogan/upo/actions/workflows/ci.yaml)
[![Lint](https://github.com/halildogan/upo/actions/workflows/lint.yaml/badge.svg)](https://github.com/halildogan/upo/actions/workflows/lint.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/halildogan/upo)](https://goreportcard.com/report/github.com/halildogan/upo)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.24-00ADD8.svg)](go.mod)

</div>

---

## The problem

Platform teams building internal developer platforms (IDPs) and multi-tenant
SaaS keep rebuilding the same plumbing: per-tenant namespaces, quotas, network
isolation, RBAC, per-environment GitOps wiring, and brittle scripts that glue in
external systems (webhooks, APIs, databases). That glue is imperative,
drift-prone, and invisible to the cluster.

**UPO turns that plumbing into first-class, declarative Kubernetes resources.**
You describe *what* a tenant, environment, or integration should be; the
operator's reconciliation loops continuously make it so — idempotently,
observably, and with safe teardown. It behaves like a mini Platform-as-a-Service
built entirely on Kubernetes primitives, following CNCF conventions
(controller-runtime, CRDs, GitOps compatibility, Prometheus/OpenTelemetry).

## What you get

- **`Tenant`** → an isolated namespace with a `ResourceQuota`, `LimitRange`,
  default `NetworkPolicy`, and admin `RoleBinding`. Suspend/resume and
  delete/orphan lifecycle built in.
- **`Environment`** → a tenant-scoped workload namespace with projected config
  and secrets, optional **Argo CD or Flux** delivery, and **ephemeral TTL**
  environments that self-destruct (perfect for PR previews).
- **`Integration`** → an external connector (webhook / REST / OAuth2 / database
  / messaging / object store) with managed credentials, a normalized connection
  `Secret`, active health probing, and **retry with exponential backoff**.

All three report rich `status.conditions` and a coarse `phase`, expose
Prometheus metrics, emit Kubernetes events, and are guarded by validating &
defaulting admission webhooks.

## Architecture

```
 platform engineer ──(kubectl / GitOps)──► Kubernetes API ──watch──► UPO manager
                                              ▲                          │
                                              │ status / events          │ reconcile
                                              └──────────────────────────┘
                                                          │
        ┌───────────────────────────────────────────────┼───────────────────────────┐
        ▼                                                 ▼                           ▼
   Tenant Reconciler                          Environment Reconciler          Integration Reconciler
   namespace · quota · limits                 workload ns · config/secrets     auth secret · connection
   networkpolicy · rbac                        Argo CD / Flux · TTL            secret · HTTP health probe
```

UPO uses the standard level-triggered loop — **observe → diff → converge →
record status** — and re-runs on every change to a managed resource plus a
periodic resync. Full diagrams (control loop, event triggers, per-CRD state
machines, ownership model) live in **[docs/architecture.md](docs/architecture.md)**.

## Quick start

> Prerequisites: a cluster (e.g. `kind`), `kubectl`, and `cert-manager` (for the
> webhooks). See **[docs/getting-started.md](docs/getting-started.md)** for the
> full walkthrough.

### Install with Helm

```bash
helm install upo charts/unified-platform-operator \
  --namespace upo-system --create-namespace
```

### Install with kubectl / kustomize

```bash
make install                                   # CRDs
make deploy IMG=ghcr.io/halildogan/upo:latest
```

### Create a tenant

```yaml
apiVersion: platform.upo.io/v1alpha1
kind: Tenant
metadata:
  name: acme
spec:
  tier: enterprise
  networkIsolation: Baseline
  quota:
    hard:
      requests.cpu: "20"
      requests.memory: 40Gi
      pods: "100"
  admins:
  - kind: Group
    name: acme-platform-admins
```

```bash
kubectl apply -f examples/tenant.yaml
kubectl get tenants
# NAME   TIER         NAMESPACE     PHASE    READY   AGE
# acme   enterprise   tenant-acme   Active   True    8s
```

### Spin up a GitOps-delivered, ephemeral preview environment

```yaml
apiVersion: platform.upo.io/v1alpha1
kind: Environment
metadata:
  name: pr-1421
  namespace: tenant-acme
spec:
  tenantRef: { name: acme }
  type: ephemeral
  ttl: 48h
  gitops: { provider: argocd, autoSync: true, prune: true }
  source:
    repoURL: https://github.com/acme/app
    revision: refs/pull/1421/head
    path: deploy/overlays/preview
```

### Wire in an external system

```yaml
apiVersion: platform.upo.io/v1alpha1
kind: Integration
metadata:
  name: stripe-events
  namespace: tenant-acme
spec:
  tenantRef: { name: acme }
  type: webhook
  endpoint: https://hooks.acme.example.com/stripe
  authSecretRef: { name: stripe-webhook-credentials }
  retryPolicy: { maxRetries: 8, backoff: exponential }
```

More in [`examples/`](examples/) (including a complete [`full-stack.yaml`](examples/full-stack.yaml)).

## The CRDs at a glance

| Kind | Scope | Reconciles into |
|------|-------|-----------------|
| `Tenant` | Cluster | Namespace, ResourceQuota, LimitRange, NetworkPolicy, RBAC |
| `Environment` | Namespaced | Workload namespace, ConfigMap, replicated Secrets, Argo CD/Flux app |
| `Integration` | Namespaced | Connection Secret, health probe, retry/backoff state |

Full field reference: **[docs/crds.md](docs/crds.md)**.

## Development

```bash
make help            # list all targets

make manifests       # regenerate CRDs/RBAC/webhook manifests (controller-gen)
make generate        # regenerate DeepCopy methods
make fmt vet lint    # format, vet, golangci-lint
make test            # unit + envtest integration tests
make build           # build the manager binary
make run             # run locally against your kube context (ENABLE_WEBHOOKS=false)
make docker-build IMG=...   # build the container image
make deploy IMG=...         # deploy to the current cluster
```

The codebase follows the kubebuilder v4 layout:

```
api/v1alpha1/      CRD Go types + DeepCopy
cmd/main.go        manager bootstrap (metrics, leader election, webhooks, tracing)
internal/controller/   Tenant / Environment / Integration reconcilers
internal/platform/     idempotent provisioning primitives (ns, quota, rbac, gitops, connector)
internal/webhook/      defaulting + validating admission webhooks
internal/metrics/      custom Prometheus metrics
pkg/conditions/        status condition helpers
config/                kustomize bases (crd, rbac, manager, webhook, certmanager, prometheus)
charts/                Helm chart
docs/ · examples/      documentation and sample manifests
```

See the annotated tree and design rationale in **[docs/architecture.md](docs/architecture.md)**.

## Observability

- **Metrics** — built-in controller-runtime metrics plus custom `upo_*` series
  (`upo_reconcile_total`, `upo_reconcile_duration_seconds`, `upo_resource_phase`,
  `upo_integration_events_*`). Served over authenticated HTTPS; a Prometheus
  `ServiceMonitor` ships in `config/prometheus`.
- **Tracing** — OpenTelemetry spans per reconcile (configure an OTLP exporter
  via `OTEL_EXPORTER_OTLP_ENDPOINT`).
- **Events & status** — Kubernetes events on state transitions; `status.conditions`
  compatible with `kubectl wait --for=condition=Ready`.

## Compatibility

- Kubernetes **1.29+**
- Go **1.24**, controller-runtime **0.20**
- GitOps: **Argo CD** and **Flux** (optional, per Environment)
- cert-manager for webhook serving certificates

## Roadmap

UPO is pre-1.0. The path from `v0.1` (MVP) to `v1.0` (HA, enterprise-ready) is
in **[ROADMAP.md](ROADMAP.md)**.

## Contributing

Contributions are welcome! Please read **[CONTRIBUTING.md](CONTRIBUTING.md)** for
the development workflow, coding standards, and the DCO sign-off requirement.

## License

Licensed under the **Apache License 2.0** — see [LICENSE](LICENSE).
