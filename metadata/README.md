# metadata

Committed inputs and gates of the generator, not part of the consumer module.

- `winmd/Windows.Win32.winmd` + `PROVENANCE.json` — the source of truth
  (`go run ./cmd/generate/ fetch-metadata` updates it).
- `diagnostics-baseline.json` — the diagnostics ratchet (CI fails on new entries).
- `win32/` — the derived IR cache, gitignored (`go run ./cmd/generate/ ingest`).

The `go.mod` here declares a Go-code-free nested module. Go module zips
exclude subtrees that carry their own `go.mod`, so consumers of
`github.com/deploymenttheory/go-bindings-win32` no longer download the 23 MB
winmd. The generator reads these files by path, so nothing imports this
module.
