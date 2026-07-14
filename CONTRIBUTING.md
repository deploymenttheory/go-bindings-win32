# Contributing

Thanks for your interest in improving this project.

## Issues

Please file bugs and feature requests on the
[issue tracker](https://github.com/deploymenttheory/go-bindings-win32/issues), filling out
the template. Small, well-described reports are genuinely useful contributions.

## Pull requests

- PR titles follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, `chore:`, …) — this is CI-enforced.
- Run `gofmt`/`go vet` and keep the build and tests green.
- By contributing you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Generated code

**Never hand-edit generated code under `bindings/`.** It is produced by the
generator and overwritten on every run — a manual edit is lost the next time
anyone regenerates, and CI's determinism gate (`git diff --exit-code` after a
clean regenerate) will fail. Fix the generator under `internal/` (or `cmd/`)
and regenerate; commit the regenerated tree alongside the generator change.
