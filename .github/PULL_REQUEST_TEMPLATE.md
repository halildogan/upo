<!-- Thanks for contributing! Please keep PRs focused and reasonably small. -->

## What does this PR do?

<!-- A short summary of the change and the motivation (the "why"). -->

## Related issues

<!-- e.g. Fixes #123 -->

## Type of change

- [ ] Bug fix
- [ ] Feature
- [ ] Docs
- [ ] Refactor / chore
- [ ] API change (CRD field added/changed)

## Checklist

- [ ] Ran `make manifests generate` and committed the results (CRDs / RBAC / webhook / DeepCopy are up to date)
- [ ] Ran `make fmt vet lint`
- [ ] Added/updated tests; `make test` passes
- [ ] Updated docs (`docs/`, `README.md`) and `CHANGELOG.md` if user-facing
- [ ] Reconciliation remains idempotent and observable via status/conditions/events
- [ ] API changes follow the versioning policy (additive + defaulted; breaking changes gated on a new version)
- [ ] Commits are signed off (DCO): `git commit -s`
