# Security Policy

## Supported versions

The Unified Platform Operator is pre-1.0. Security fixes are applied to the
latest `0.x` minor release line. Older pre-releases are not maintained.

| Version | Supported |
|---------|-----------|
| `0.1.x` | ✅ |
| `< 0.1` | ❌ |

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately using one of:

1. **GitHub Private Vulnerability Reporting** — on the repository, go to the
   **Security** tab → **Report a vulnerability**. This is the preferred channel.
2. **Email** — `security@upo.platform.upo.io` with the details below.

Include, where possible:

- A description of the issue and its impact.
- Affected version(s) / commit SHA.
- Reproduction steps or a proof of concept.
- Any suggested remediation.

## What to expect

- **Acknowledgement** within 3 business days.
- **Triage & severity assessment** within 7 business days.
- Coordinated disclosure: we will agree on a disclosure timeline with the
  reporter, aiming to ship a fix within 90 days (sooner for high severity).
- Credit in the release notes for responsibly-disclosed reports (opt-out
  available).

## Scope

In scope: the operator (controllers, webhooks, RBAC defaults), the published
container image, the Helm chart, and the kustomize manifests in this repository.

Out of scope: vulnerabilities in upstream dependencies (report those upstream),
misconfigurations in a user's own cluster, and issues requiring cluster-admin to
already be compromised.
