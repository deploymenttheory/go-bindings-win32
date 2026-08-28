//go:build windows

// Package abi is the struct-layout gate: `go run ./cmd/generate abitest`
// writes one generated abi_<namespace>_test.go per namespace asserting the
// size and every field offset of every emitted struct against the C layout
// computed from the metadata (the LLP64 model amd64 and arm64 share). The
// tables are compile-time constants, so this is cheap to run and catches a
// layout regression on any winmd bump.
package abi

import "testing"

// abiCase is one size or offset assertion.
type abiCase struct {
	name      string
	got, want uintptr
}

// checkABI reports every mismatching assertion in cases.
func checkABI(t *testing.T, cases []abiCase) {
	t.Helper()
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}
