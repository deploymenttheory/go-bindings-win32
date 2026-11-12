package rawwin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// WinMD omits Optional for this server parameter, although the native API
// documents NULL as the local computer. Exercise the emitted marshaling and
// constrain the correction to this function's input server parameter.
func TestDsRoleServerAllowsNULL(t *testing.T) {
	for _, tc := range []struct {
		name, namespace, function, parameter string
		out, reserved, nullable              bool
	}{
		{name: "local computer", namespace: "Networking.ActiveDirectory", function: "DsRoleGetPrimaryDomainInformation", parameter: "lpServer", nullable: true},
		{name: "other namespace", namespace: "Test.Other", function: "DsRoleGetPrimaryDomainInformation", parameter: "lpServer"},
		{name: "other function", namespace: "Networking.ActiveDirectory", function: "OtherFunction", parameter: "lpServer"},
		{name: "other parameter", namespace: "Networking.ActiveDirectory", function: "DsRoleGetPrimaryDomainInformation", parameter: "text"},
		{name: "changed output contract", namespace: "Networking.ActiveDirectory", function: "DsRoleGetPrimaryDomainInformation", parameter: "lpServer", out: true},
		{name: "changed reserved contract", namespace: "Networking.ActiveDirectory", function: "DsRoleGetPrimaryDomainInformation", parameter: "lpServer", reserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metaDir, outDir := t.TempDir(), t.TempDir()
			server := param(tc.parameter, pwstrType(), in)
			server.IsOut, server.IsReserved = tc.out, tc.reserved
			for _, meta := range []*win32meta.NamespaceMeta{
				syntheticNamespaces()[0],
				{Namespace: tc.namespace, Functions: []win32meta.Function{
					function(tc.function, "NETAPI32.dll", native("UInt32"), server),
				}},
			} {
				if err := win32meta.Write(metaDir, meta); err != nil {
					t.Fatal(err)
				}
			}
			registry, err := pipeline.LoadAll(metaDir)
			if err != nil {
				t.Fatal(err)
			}
			generator := New(registry, testModulePath, outDir)
			if _, err := generator.EmitAll(nil); err != nil {
				t.Fatal(err)
			}
			for _, meta := range registry.Namespaces {
				for _, function := range meta.Functions {
					if function.Name == tc.function && function.Params[0].IsOptional {
						t.Error("gather mutated shared metadata")
					}
				}
			}
			path := strings.ToLower(strings.ReplaceAll(tc.namespace, ".", "/"))
			source := readTree(t, outDir)[path+"/"+filepath.Base(path)+"_functions.go"]
			if tc.out || tc.reserved {
				if strings.Contains(source, "func "+tc.function+"(") || len(generator.Diagnostics) == 0 {
					t.Fatal("changed native contract must be skipped with a diagnostic")
				}
				return
			}
			if source == "" {
				t.Fatal("function source was not emitted")
			}
			wantType, conversion := "string", "UTF16Ptr"
			if tc.nullable {
				wantType, conversion = "*string", "UTF16PtrOrNil"
			}
			for _, want := range []string{
				"func " + tc.function + "(" + tc.parameter + " " + wantType + ") uint32",
				"_" + tc.parameter + " := win32." + conversion + "(" + tc.parameter + ")",
				"uintptr(unsafe.Pointer(_" + tc.parameter + "))",
				"return uint32(r1)",
			} {
				if !strings.Contains(source, want) {
					t.Errorf("emitted function is missing %q", want)
				}
			}
		})
	}
}
