package rawwin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

func TestLDAPClientCertificateOptionPassesCallbackByValue(t *testing.T) {
	for _, scenario := range []string{"native", "other-namespace", "missing-delegate", "changed-setter", "name-collision"} {
		t.Run(scenario, func(t *testing.T) {
			namespace := "Networking.Ldap"
			if scenario == "other-namespace" {
				namespace = "Test.Other"
			}
			meta := &win32meta.NamespaceMeta{
				Namespace: namespace,
				Structs:   map[string]win32meta.Struct{"LDAP": {Fields: []win32meta.StructField{field("value", native("UInt32"))}}},
				Delegates: map[string]win32meta.FuncPointer{"QUERYCLIENTCERT": {Return: native("Byte")}},
				Constants: []win32meta.Constant{{Name: "LDAP_OPT_CLIENT_CERTIFICATE", Type: native("UInt32"), Value: "128", ValueKind: "UInt"}},
				Functions: []win32meta.Function{function("ldap_set_optionW", "WLDAP32.dll", native("UInt32"),
					param("ld", pointerTo(apiRef(namespace, "LDAP", "Struct")), in, out),
					param("option", native("Int32"), in), param("invalue", voidPtrType(), in))},
			}
			switch scenario {
			case "missing-delegate":
				meta.Delegates = nil
			case "changed-setter":
				meta.Functions[0].Params[2].Type = native("UInt32")
			case "name-collision":
				meta.Functions = append(meta.Functions, function("Ldap_set_client_certificate", "OTHER.dll", native("Void")))
			}
			metaDir, outDir := t.TempDir(), t.TempDir()
			if err := win32meta.Write(metaDir, meta); err != nil {
				t.Fatal(err)
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
			const signature = "func Ldap_set_client_certificate(ld *LDAP, callback QUERYCLIENTCERT) uint32"
			if scenario != "native" {
				if strings.Contains(source, signature) {
					t.Fatal("helper emitted despite incompatible source contract")
				}
				return
			}
			for _, want := range []string{
				signature,
				"syscall.SyscallN(procLdap_set_optionW.Addr(), uintptr(unsafe.Pointer(ld)), uintptr(LDAP_OPT_CLIENT_CERTIFICATE), uintptr(callback))",
				"func Ldap_set_optionW(ld *LDAP, option int32, invalue unsafe.Pointer) uint32",
			} {
				if !strings.Contains(source, want) {
					t.Errorf("missing emitted contract %q", want)
				}
			}
			for _, forbidden := range []string{"unsafe.Pointer(callback)", "unsafe.Pointer(&callback)", `NewProc("ldap_set_client_certificate")`} {
				if strings.Contains(source, forbidden) {
					t.Errorf("incorrect callback dispatch %q", forbidden)
				}
			}
			coverage := generator.Coverage()
			if len(coverage) != 1 || coverage[0].Functions != 1 || coverage[0].FunctionsEmitted != 1 {
				t.Fatalf("helper inflated native export coverage: %+v", coverage)
			}
		})
	}
}
