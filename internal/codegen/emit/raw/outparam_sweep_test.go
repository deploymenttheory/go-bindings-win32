package rawwin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestGeneratedTreeHeapEscapesOutParams sweeps the committed generated tree
// for the out-param invariant: elevated [out,retval] locals must be
// heap-escaped through win32.OutParam, never passed as stack addresses
// (`unsafe.Pointer(&_local)`), because a native callee that reenters Go via
// a consumer callback can move the goroutine's stack and strand the pointer
// (see bindings/runtime/win32/outparam.go).
func TestGeneratedTreeHeapEscapesOutParams(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "bindings", "win32")
	stackPattern := regexp.MustCompile(`unsafe\.Pointer\(&_[A-Za-z0-9_]+\)`)

	var files, outParamSites, violations int
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		files++
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)
		outParamSites += strings.Count(text, "win32.OutParam(unsafe.Pointer(")
		if match := stackPattern.FindString(text); match != "" {
			violations++
			if violations <= 5 {
				t.Errorf("%s: stack out-param %q", path, match)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking generated tree: %v", err)
	}
	t.Logf("%d files, %d OutParam sites, %d stack-pattern violations", files, outParamSites, violations)
	if violations > 0 {
		t.Fatalf("%d generated files pass stack addresses as out-params", violations)
	}
	if outParamSites == 0 {
		t.Fatal("no win32.OutParam sites found — sweep is looking at the wrong tree or the emitter regressed")
	}
}
