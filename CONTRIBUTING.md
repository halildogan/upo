# Contributing to the Unified Platform Operator

Thanks for your interest in improving UPO! This document explains how to set up
your environment, the standards we hold code to, and how to get a change merged.

## Code of Conduct

This project follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md).
By participating you agree to uphold it. Report unacceptable behavior to the
maintainers.

## Ways to contribute

- **Bug reports** — open an issue with a minimal reproduction (CR YAML, operator
  version, `kubectl get <kind> -o yaml`, and manager logs).
- **Feature requests** — describe the platform problem first, then the proposed
  API/behavior. Check the [ROADMAP](ROADMAP.md) for alignment.
- **Code & docs** — see the workflow below.

## Development setup

```bash
git clone https://github.com/halildogan/upo
cd unified-platform-operator
go mod download

# Tooling (installed into ./bin on demand by the Makefile):
make controller-gen kustomize envtest golangci-lint
```

Iterate locally against a kind cluster:

```bash
make install
ENABLE_WEBHOOKS=false make run
```

## Making a change

1. **Branch** from `main`: `git checkout -b feat/short-description`.
2. **Edit API types?** Re-run code generation and commit the results:
   ```bash
   make manifests generate
   ```
   CI fails if generated files (`zz_generated.deepcopy.go`, CRDs, RBAC, webhook
   manifests) are out of date.
3. **Keep it idempotent.** Reconcilers must converge on every call; use
   `CreateOrUpdate`/server-side apply, never assume prior state.
4. **Update status, not just the world.** New behavior should be observable via
   `status.conditions`/`phase`, events, and (where meaningful) metrics.
5. **Test.** Add unit tests for pure logic and envtest specs for reconcile
   behavior:
   ```bash
   make fmt vet lint
   make test
   ```
6. **Docs.** Update `docs/` and `README.md` for user-facing changes; update
   `ROADMAP.md` if you complete a milestone item.

## Coding standards

- Go 1.24, `gofmt`/`goimports` clean, `golangci-lint` green (`.golangci.yml`).
- Follow controller-runtime idioms and the existing package layout.
- Keep provisioning logic in `internal/platform`; keep reconcilers thin and
  focused on orchestration + status.
- Public API changes (CRD fields) require: validation markers, defaults where
  sensible, doc comments (they become CRD descriptions), and a `docs/crds.md`
  update. Breaking changes require a new API version + conversion.
- Conventional Commit messages are encouraged (`feat:`, `fix:`, `docs:`…).

## Sign-off (DCO)

All commits must be signed off under the
[Developer Certificate of Origin](https://developercertificate.org/):

```bash
git commit -s -m "feat: add environment promotion gates"
```

## Pull requests

- Keep PRs focused and reasonably small; split unrelated changes.
- Fill in the PR description: what, why, and how it was tested.
- Ensure CI (test, lint, build, helm) is green.
- A maintainer review is required before merge; address review comments by
  pushing new commits (we squash on merge).

## Release process (maintainers)

Tagging `vX.Y.Z` triggers `.github/workflows/release.yaml`, which builds and
pushes the multi-arch image to GHCR, renders the `install.yaml` bundle, packages
the Helm chart, and publishes a GitHub Release.

## License

By contributing, you agree that your contributions are licensed under the
project's [Apache 2.0 License](LICENSE).
