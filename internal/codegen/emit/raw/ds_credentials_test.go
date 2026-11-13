package rawwin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

func TestDsCredentialUTF16CompanionPreservesCallerBuffers(t *testing.T) {
	for _, scenario := range []string{"native", "other-namespace", "changed-parameter", "changed-direction", "changed-optional", "changed-return", "last-error", "other-dll", "name-collision"} {
		t.Run(scenario, func(t *testing.T) {
			namespace := "Networking.ActiveDirectory"
			if scenario == "other-namespace" {
				namespace = "Test.Other"
			}
			optional := func(p *win32meta.Param) { p.IsOptional = true }
			fn := withUnsuffixed(function("DsMakePasswordCredentialsW", "NTDSAPI.dll", native("UInt32"), param("User", pwstrType(), in, optional), param("Domain", pwstrType(), in, optional), param("Password", pwstrType(), in, optional), param("pAuthIdentity", voidDoublePtrType(), out)), "DsMakePasswordCredentials")
			switch scenario {
			case "changed-parameter":
				fn.Params[2].Type = native("UInt32")
			case "changed-direction":
				fn.Params[2].IsOut = true
			case "changed-optional":
				fn.Params[2].IsOptional = false
			case "changed-return":
				fn.Return = native("UInt64")
			case "last-error":
				fn.SetLastError = true
			case "other-dll":
				fn.DLL = "OTHER.dll"
			}
			meta := &win32meta.NamespaceMeta{Namespace: namespace, Functions: []win32meta.Function{fn}}
			if scenario == "name-collision" {
				meta.Functions = append(meta.Functions, function("DsMakePasswordCredentialsUTF16", "OTHER.dll", native("Void")))
			}
			metaDir, outDir := t.TempDir(), t.TempDir()
			for _, m := range []*win32meta.NamespaceMeta{syntheticNamespaces()[0], meta} {
				if err := win32meta.Write(metaDir, m); err != nil {
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
			path := strings.ToLower(strings.ReplaceAll(namespace, ".", "/"))
			source := readTree(t, outDir)[path+"/"+filepath.Base(path)+"_functions.go"]
			const signature = "func DsMakePasswordCredentialsUTF16(User foundation.PWSTR, Domain foundation.PWSTR, Password foundation.PWSTR, pAuthIdentity *unsafe.Pointer) uint32"
			if scenario != "native" {
				if strings.Contains(source, signature) {
					t.Fatal("companion emitted for changed native contract")
				}
				return
			}
			for _, want := range []string{signature, "func DsMakePasswordCredentials(User *string, Domain *string, Password *string, pAuthIdentity *unsafe.Pointer) uint32", "syscall.SyscallN(procDsMakePasswordCredentials.Addr(), uintptr(unsafe.Pointer(User)), uintptr(unsafe.Pointer(Domain)), uintptr(unsafe.Pointer(Password)), uintptr(unsafe.Pointer(pAuthIdentity)))"} {
				if !strings.Contains(source, want) {
					t.Errorf("missing emitted contract %q", want)
				}
			}
			_, body, found := strings.Cut(source, signature)
			if !found || strings.Contains(body, "UTF16Ptr") || strings.Contains(source, `NewProc("DsMakePasswordCredentialsUTF16")`) {
				t.Fatal("companion allocates hidden strings or invents an export")
			}
			for _, coverage := range generator.Coverage() {
				if coverage.Namespace == namespace && coverage.FunctionsEmitted != 1 {
					t.Fatalf("companion inflated metadata coverage: %+v", coverage)
				}
			}
		})
	}
}
