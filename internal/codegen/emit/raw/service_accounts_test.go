package rawwin

import (
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

func TestServiceAccountUTF16CompanionContract(t *testing.T) {
	for _, scenario := range []string{"native", "other-namespace", "changed-type", "changed-direction", "required-server", "optional-account", "changed-return", "last-error", "other-dll", "name-collision"} {
		t.Run(scenario, func(t *testing.T) {
			namespace := "NetworkManagement.NetManagement"
			if scenario == "other-namespace" {
				namespace = "Other"
			}
			optional := func(p *win32meta.Param) { p.IsOptional = true }
			fn := function("NetAddServiceAccount", "LOGONCLI.dll", apiRef(foundation, "NTSTATUS", "Typedef"), param("ServerName", pwstrType(), in, optional), param("AccountName", pwstrType(), in), param("Password", pwstrType(), in), param("Flags", native("UInt32"), in))
			switch scenario {
			case "changed-type":
				fn.Params[2].Type = native("UInt32")
			case "changed-direction":
				fn.Params[2].IsOut = true
			case "required-server":
				fn.Params[0].IsOptional = false
			case "optional-account":
				fn.Params[1].IsOptional = true
			case "changed-return":
				fn.Return = native("UInt32")
			case "last-error":
				fn.SetLastError = true
			case "other-dll":
				fn.DLL = "OTHER.dll"
			}
			meta := &win32meta.NamespaceMeta{Namespace: namespace, Functions: []win32meta.Function{fn}}
			if scenario == "name-collision" {
				meta.Functions = append(meta.Functions, function("NetAddServiceAccountUTF16", "OTHER.dll", native("Void")))
			}
			foundationMeta := syntheticNamespaces()[0]
			foundationMeta.Typedefs["NTSTATUS"] = win32meta.Typedef{Underlying: native("Int32")}
			metadata, out := t.TempDir(), t.TempDir()
			for _, m := range []*win32meta.NamespaceMeta{foundationMeta, meta} {
				if err := win32meta.Write(metadata, m); err != nil {
					t.Fatal(err)
				}
			}
			registry, err := pipeline.LoadAll(metadata)
			if err != nil {
				t.Fatal(err)
			}
			generator := New(registry, testModulePath, out)
			if _, err = generator.EmitAll(nil); err != nil {
				t.Fatal(err)
			}
			path := strings.ToLower(strings.ReplaceAll(namespace, ".", "/"))
			parts := strings.Split(path, "/")
			source := readTree(t, out)[path+"/"+parts[len(parts)-1]+"_functions.go"]
			const signature = "func NetAddServiceAccountUTF16(ServerName foundation.PWSTR, AccountName foundation.PWSTR, Password foundation.PWSTR, Flags uint32) foundation.NTSTATUS"
			if scenario != "native" {
				if strings.Contains(source, signature) {
					t.Fatal("emitted changed contract")
				}
				return
			}
			for _, want := range []string{signature, "func NetAddServiceAccount(ServerName *string, AccountName string, Password string, Flags uint32) foundation.NTSTATUS", `modLOGONCLI = win32.NewDLL("LOGONCLI.dll")`, "return foundation.NTSTATUS(r1)"} {
				if !strings.Contains(source, want) {
					t.Error("missing", want)
				}
			}
			_, body, _ := strings.Cut(source, signature)
			if strings.Contains(body, "UTF16Ptr") || strings.Contains(source, `NewProc("NetAddServiceAccountUTF16")`) {
				t.Fatal("hidden allocation or invented export")
			}
			for _, coverage := range generator.Coverage() {
				if coverage.Namespace == namespace && coverage.FunctionsEmitted != 1 {
					t.Fatal("inflated coverage", coverage)
				}
			}
		})
	}
}
