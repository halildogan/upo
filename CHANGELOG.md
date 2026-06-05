# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). While the
project is `0.x`, minor versions may contain breaking changes (see
[ROADMAP.md](ROADMAP.md) and the API versioning policy).

## [Unreleased]

## [0.1.0] — MVP

Initial public release of the Unified Platform Operator.

### Added
- **CRDs** (`platform.upo.io/v1alpha1`):
  - `Tenant` (cluster-scoped) — provisions an isolated namespace with
    `ResourceQuota`, `LimitRange`, default `NetworkPolicy`, and admin
    `RoleBinding`; tiers, suspend/resume, and delete/orphan lifecycle.
  - `Environment` (namespaced) — tenant-scoped workload namespace, config and
    secret projection, optional Argo CD / Flux delivery, and ephemeral TTL
    environments.
  - `Integration` (namespaced) — external connector with managed credentials, a
    normalized connection `Secret`, HTTP health probing, and retry/backoff.
- **Controllers** — three controller-runtime reconcilers with finalizers, owner
  references, status conditions, events, idempotent provisioning, exponential
  backoff, custom Prometheus metrics, and OpenTelemetry tracing hooks.
- **Admission webhooks** — defaulting and validating webhooks for all three
  kinds (immutability, cross-field, and URL/TTL rules).
- **Operations** — secure metrics (authn/authz), leader election, health/ready
  probes.
- **Packaging** — kustomize config (`config/`), a self-contained Helm chart with
  cert-manager wiring, examples, and documentation.
- **Quality** — unit tests, an envtest integration suite, an e2e scaffold, and
  GitHub Actions CI (test, lint, build, helm), plus a release workflow.

[Unreleased]: https://github.com/halildogan/upo/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/halildogan/upo/releases/tag/v0.1.0
