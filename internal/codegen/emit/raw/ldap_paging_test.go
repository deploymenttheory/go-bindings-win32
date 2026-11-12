package rawwin

import (
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

func TestLDAPPagingReturnChecksHandle(t *testing.T) {
	for _, namespace := range []string{"Networking.Ldap", "Test.Other"} {
		for _, name := range []string{"ldap_search_init_page", "ldap_search_init_pageA", "ldap_search_init_pageW", "other_function"} {
			t.Run(namespace+"."+name, func(t *testing.T) {
				metaDir, outDir := t.TempDir(), t.TempDir()
				meta := &win32meta.NamespaceMeta{
					Namespace: namespace,
					Typedefs:  map[string]win32meta.Typedef{"PLDAPSearch": {Underlying: native("IntPtr")}},
					Functions: []win32meta.Function{withLastError(function(name, "WLDAP32.dll", apiRef(namespace, "PLDAPSearch", "Typedef")))},
				}
				if err := win32meta.Write(metaDir, meta); err != nil {
					t.Fatal(err)
				}
				registry, err := pipeline.LoadAll(metaDir)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := New(registry, testModulePath, outDir).EmitAll(nil); err != nil {
					t.Fatal(err)
				}
				source := ""
				for path, value := range readTree(t, outDir) {
					if strings.HasSuffix(path, "_functions.go") {
						source += value
					}
				}
				curated := namespace == "Networking.Ldap" && strings.HasPrefix(name, "ldap_search_init_page")
				if curated {
					if !strings.Contains(source, "if ret == 0 {") || !strings.Contains(source, "return ret, nil") {
						t.Fatal("paging return must use the NULL handle failure sentinel")
					}
				} else if !strings.Contains(source, "if e1 != 0 {") {
					t.Fatal("unrelated advisory return contract changed")
				}
			})
		}
	}
}
