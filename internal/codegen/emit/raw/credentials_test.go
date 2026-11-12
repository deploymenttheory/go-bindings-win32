package rawwin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// ldap_bind_sW's cred is a method-dependent pointer: Negotiate accepts a
// SEC_WINNT_AUTH_IDENTITY structure, whereas SIMPLE accepts a password string.
// Converting either caller pointer into a fresh Go string destroys that contract.
func TestLDAPBindCredentialsPreserveNativePointer(t *testing.T) {
	for _, tc := range []struct {
		name, namespace, function string
		preserve                  bool
	}{
		{"synchronous credentials", "Networking.Ldap", "ldap_bind_sW", true},
		{"asynchronous simple bind", "Networking.Ldap", "ldap_bindW", false},
		{"different namespace", "Test.Other", "ldap_bind_sW", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metaDir, outDir := t.TempDir(), t.TempDir()
			for _, meta := range []*win32meta.NamespaceMeta{
				syntheticNamespaces()[0],
				{Namespace: tc.namespace, Functions: []win32meta.Function{
					function(tc.function, "WLDAP32.dll", native("UInt32"),
						param("dn", pwstrType(), in, optional),
						param("cred", pwstrType(), in, optional),
						param("method", native("UInt32"), in)),
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
			if !strings.Contains(source, "dn *string") || !strings.Contains(source, "win32.UTF16PtrOrNil(dn)") {
				t.Fatal("ordinary optional DN shaping changed")
			}
			if tc.preserve {
				for _, want := range []string{"cred foundation.PWSTR", "uintptr(unsafe.Pointer(cred))"} {
					if !strings.Contains(source, want) {
						t.Errorf("credential pointer must reach the native call unchanged; missing %q", want)
					}
				}
				if strings.Contains(source, "win32.UTF16PtrOrNil(cred)") {
					t.Error("structure credentials must not be marshaled as a Go string")
				}
			} else if !strings.Contains(source, "cred *string") || !strings.Contains(source, "win32.UTF16PtrOrNil(cred)") {
				t.Error("ordinary optional string shaping changed")
			}
		})
	}
}
