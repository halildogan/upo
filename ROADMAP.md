# Roadmap

This roadmap tracks the evolution of the Unified Platform Operator from MVP to a
production-grade, enterprise-ready control plane. Versions are additive; each
builds on the last. Dates are intentionally omitted — milestones ship when their
exit criteria (tests green, docs written, upgrade path verified) are met.

> Status legend: ✅ done · 🚧 in progress · ⬜ planned

---

## v0.1 — MVP ✅ (this release)

The walking skeleton: real CRDs, real controllers, runnable on a local cluster.

- ✅ `Tenant`, `Environment`, `Integration` CRDs (`platform.upo.io/v1alpha1`) with
  OpenAPI validation, defaults, printer columns, and status subresources.
- ✅ Three controller-runtime reconcilers with finalizers, owner references,
  status conditions, events, and idempotent `CreateOrUpdate` provisioning.
- ✅ Tenant → namespace + ResourceQuota + LimitRange + NetworkPolicy + admin RBAC.
- ✅ Environment → workload namespace + config/secret projection + ephemeral TTL.
- ✅ Integration → credential resolution + connection Secret + HTTP health probe
  with retry/backoff.
- ✅ Defaulting & validating admission webhooks; secure metrics; leader election.
- ✅ Local `kind` support, Helm chart, kustomize install, envtest + e2e scaffolding.

## v0.2 — Multi-tenancy hardening ⬜

Make tenancy boundaries airtight and the status model richer.

- ⬜ Tenant-scoped `ResourceQuota` scopes & priority classes; default
  `PodSecurity` admission labels per tier.
- ⬜ Hierarchical tenants (parent/child) and namespace pooling strategies.
- ⬜ Richer `Environment`/`Integration` roll-up into `Tenant.status` (health
  aggregation, not just counts).
- ⬜ Conversion webhook scaffolding to unblock future API versions.
- ⬜ Cross-tenant policy guardrails (deny references across tenant boundaries).

## v0.3 — Integration framework ⬜

Turn `Integration` from "connector record + probe" into an extensible delivery
framework.

- ⬜ Pluggable connector providers (interface + registry) beyond HTTP: SQL,
  AMQP/Kafka, S3-compatible object stores with real connectivity checks.
- ⬜ Event delivery pipeline: subscribe to platform events, deliver with the
  declared retry/rate-limit policy, and populate `deliveredEvents`/`failedEvents`.
- ⬜ OAuth2 client-credentials token lifecycle management.
- ⬜ Secret rotation watch → automatic connection Secret refresh.

## v0.4 — GitOps & environment promotion ⬜

- ⬜ First-class Argo CD `ApplicationSet` and Flux `Kustomization` health
  surfacing (read back sync waves, degraded resources).
- ⬜ Environment promotion flows (dev → staging → prod) with gates.
- ⬜ Preview-environment automation hooks for PR open/close webhooks.
- ⬜ Drift detection & reporting for non-GitOps environments.

## v0.5 — Observability & SLOs ⬜

- ⬜ Curated Grafana dashboards and Prometheus alerting rules shipped in-chart.
- ⬜ End-to-end OpenTelemetry traces (reconcile → provisioning → external calls)
  with exemplars linking metrics to traces.
- ⬜ Reconcile SLO metrics (time-to-Ready per resource kind) and burn-rate alerts.
- ⬜ Structured audit logging of all provisioning actions.

## v0.6 — Developer platform enablement ⬜

- ⬜ Service templates / golden paths (`ServiceTemplate` CRD) for self-service.
- ⬜ CI/CD bootstrapping helpers (repo + pipeline scaffolding per environment).
- ⬜ A lightweight CLI (`upoctl`) for tenant/environment self-service.
- ⬜ Backstage plugin / catalog entity exposure.

## v1.0 — Production & enterprise readiness ⬜

- ⬜ API graduation to `v1beta1`/`v1` with a tested conversion path.
- ⬜ HA controller hardening: tuned work-queue rate limits, sharding for large
  fleets, multi-cluster control plane (push model).
- ⬜ Fault-tolerance test matrix (chaos, apiserver flaps, partial outages).
- ⬜ Full security review: least-privilege RBAC audit, supply-chain (SLSA,
  signed images, SBOM), CIS-aligned defaults.
- ⬜ Documented upgrade/downgrade and backup/restore procedures.
- ⬜ Conformance test suite and a stability/deprecation policy.

---

## Non-goals

- Replacing Argo CD / Flux — UPO **delegates** to them, it does not reimplement
  GitOps reconciliation of arbitrary manifests.
- Being a general-purpose service mesh or CNI — network isolation is expressed
  via standard `NetworkPolicy`, enforced by the cluster's CNI.
- Cloud-provider IaC (VPCs, managed databases) — that belongs to Crossplane/
  Terraform; UPO can *reference* such resources via `Integration`.
