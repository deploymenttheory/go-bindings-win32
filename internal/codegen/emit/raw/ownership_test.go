package rawwin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Exercise the emitted syscall, not just the parameter classifier: freeing a
// UTF-16 copy would pass Go-owned memory to the LDAP heap and leak the original.
func TestLDAPMemoryFreePreservesNativePointer(t *testing.T) {
	for _, tc := range []struct {
		name, namespace, function, parameter string
		preserve                             bool
	}{
		{"LDAP allocation", "Networking.Ldap", "ldap_memfreeW", "Block", true},
		{"ordinary LDAP string", "Networking.Ldap", "ldap_dn2ufnW", "dn", false},
		{"different namespace", "Test.Other", "ldap_memfreeW", "Block", false},
		{"different parameter", "Networking.Ldap", "ldap_memfreeW", "text", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metaDir, outDir := t.TempDir(), t.TempDir()
			for _, meta := range []*win32meta.NamespaceMeta{
				syntheticNamespaces()[0],
				{Namespace: tc.namespace, Functions: []win32meta.Function{
					function(tc.function, "WLDAP32.dll", native("Void"), param(tc.parameter, pwstrType(), in)),
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
			if _, err := New(registry, testModulePath, outDir).EmitAll(nil); err != nil {
				t.Fatal(err)
			}
			path := strings.ToLower(strings.ReplaceAll(tc.namespace, ".", "/"))
			source := readTree(t, outDir)[path+"/"+filepath.Base(path)+"_functions.go"]
			if source == "" {
				t.Fatal("function source was not emitted")
			}
			if tc.preserve {
				for _, want := range []string{"Block foundation.PWSTR", "uintptr(unsafe.Pointer(Block))"} {
					if !strings.Contains(source, want) {
						t.Errorf("native allocation must reach the deallocator unchanged; missing %q", want)
					}
				}
				if strings.Contains(source, "win32.UTF16Ptr") {
					t.Error("deallocator must not receive a newly allocated UTF-16 copy")
				}
			} else if !strings.Contains(source, tc.parameter+" string") || !strings.Contains(source, "win32.UTF16Ptr("+tc.parameter+")") {
				t.Error("ordinary input string shaping changed")
			}
		})
	}
}
