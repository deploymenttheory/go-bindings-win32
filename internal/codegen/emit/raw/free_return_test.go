package rawwin

import (
	"slices"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

func TestMemoryFreeReturnContract(t *testing.T) {
	dir := t.TempDir()
	meta := &win32meta.NamespaceMeta{Namespace: foundation, Typedefs: map[string]win32meta.Typedef{
		"HLOCAL":  {Underlying: pointerTo(native("Void")), InvalidValues: []string{"-1", "0"}},
		"HGLOBAL": {Underlying: pointerTo(native("Void")), InvalidValues: []string{"-1", "0"}},
	}}
	if err := win32meta.Write(dir, meta); err != nil {
		t.Fatal(err)
	}
	registry, err := pipeline.LoadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	g := New(registry, testModulePath, t.TempDir())
	for _, tc := range []struct {
		namespace, name, handle string
		checks                  []string
	}{
		{foundation, "LocalFree", "HLOCAL", []string{"ret != 0"}},
		{foundation, "GlobalFree", "HGLOBAL", []string{"ret != 0"}},
		{"System.Memory", "LocalAlloc", "HLOCAL", []string{"ret == ^foundation.HLOCAL(0)", "ret == 0"}},
		{"System.Memory", "GlobalAlloc", "HGLOBAL", []string{"ret == ^foundation.HGLOBAL(0)", "ret == 0"}},
		{"Test.Other", "LocalFree", "HLOCAL", []string{"ret == ^foundation.HLOCAL(0)", "ret == 0"}},
	} {
		t.Run(tc.namespace+"."+tc.name, func(t *testing.T) {
			resolved := typemap.Resolved{GoType: "foundation." + tc.handle, Kind: typemap.KindHandleTypedef, TypedefApi: foundation, TypedefName: tc.handle}
			fn := withLastError(function(tc.name, "KERNEL32.dll", apiRef(foundation, tc.handle, "Typedef")))
			var model view.FunctionModel
			_, _, ok := g.buildReturnShape(&model, &win32meta.NamespaceMeta{Namespace: tc.namespace}, &fn, resolved)
			if !ok || model.ReturnKind != view.RetValErr || model.ReturnSig != "(foundation."+tc.handle+", error)" {
				t.Fatalf("changed public return shape: %+v", model)
			}
			if !slices.Equal(model.FailureChecks, tc.checks) {
				t.Fatalf("failure checks = %v, want %v", model.FailureChecks, tc.checks)
			}
		})
	}
}
