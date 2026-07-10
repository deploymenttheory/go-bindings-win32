package winmd

import (
	"os"
	"path/filepath"
	"testing"
)

// testFile opens the committed Windows.Win32.winmd once per test binary.
func testFile(t *testing.T) *File {
	t.Helper()
	path := filepath.Join("..", "..", "metadata", "winmd", "Windows.Win32.winmd")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("committed winmd not present: %v", err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return file
}

func TestOpenTableCounts(t *testing.T) {
	file := testFile(t)

	if file.Version == "" {
		t.Error("empty metadata version string")
	}
	counts := map[string]int{
		"TypeDefs":         len(file.Tables.TypeDefs),
		"TypeRefs":         len(file.Tables.TypeRefs),
		"Fields":           len(file.Tables.Fields),
		"Methods":          len(file.Tables.Methods),
		"Params":           len(file.Tables.Params),
		"CustomAttributes": len(file.Tables.CustomAttributes),
		"ImplMaps":         len(file.Tables.ImplMaps),
		"Constants":        len(file.Tables.Constants),
		"NestedClasses":    len(file.Tables.NestedClasses),
		"InterfaceImpls":   len(file.Tables.InterfaceImpls),
		"ModuleRefs":       len(file.Tables.ModuleRefs),
	}
	t.Logf("version=%s counts=%v", file.Version, counts)
	// The Win32 metadata is huge; sanity floors catch gross decode failures.
	if counts["TypeDefs"] < 10000 {
		t.Errorf("TypeDefs = %d, want >= 10000", counts["TypeDefs"])
	}
	if counts["Methods"] < 50000 {
		t.Errorf("Methods = %d, want >= 50000", counts["Methods"])
	}
	if counts["ImplMaps"] < 10000 {
		t.Errorf("ImplMaps = %d, want >= 10000", counts["ImplMaps"])
	}
}

func TestWellKnownTypes(t *testing.T) {
	file := testFile(t)

	var foundApis, foundHandle, foundIUnknown bool
	for _, typeDef := range file.Tables.TypeDefs {
		switch {
		case typeDef.Name == "Apis" && typeDef.Namespace == "Windows.Win32.System.Threading":
			foundApis = true
			if typeDef.MethodFirst >= typeDef.MethodEnd {
				t.Error("Threading Apis class has no methods")
			}
		case typeDef.Name == "HANDLE" && typeDef.Namespace == "Windows.Win32.Foundation":
			foundHandle = true
		case typeDef.Name == "IUnknown" && typeDef.Namespace == "Windows.Win32.System.Com":
			foundIUnknown = true
		}
	}
	if !foundApis {
		t.Error("missing Windows.Win32.System.Threading.Apis")
	}
	if !foundHandle {
		t.Error("missing Windows.Win32.Foundation.HANDLE")
	}
	if !foundIUnknown {
		t.Error("missing Windows.Win32.System.Com.IUnknown")
	}
}

func TestMethodNamesResolve(t *testing.T) {
	file := testFile(t)

	found := map[string]bool{}
	for _, typeDef := range file.Tables.TypeDefs {
		if typeDef.Name != "Apis" || typeDef.Namespace != "Windows.Win32.System.Threading" {
			continue
		}
		for row := typeDef.MethodFirst; row < typeDef.MethodEnd; row++ {
			found[file.Tables.Methods[row-1].Name] = true
		}
	}
	for _, want := range []string{"CreateEventW", "SetEvent", "WaitForSingleObject", "CreateThread"} {
		if !found[want] {
			t.Errorf("Threading Apis missing method %s", want)
		}
	}
}
