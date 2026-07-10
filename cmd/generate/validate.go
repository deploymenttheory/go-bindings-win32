package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// runValidate performs structural integrity checks over the committed
// metadata: dangling type references, unresolvable typedefs, empty DLL
// imports, malformed enums. Errors fail the process; warnings report.
func runValidate(args []string) error {
	flags := flag.NewFlagSet("validate", flag.ExitOnError)
	metadataDir := flags.String("metadata", filepath.Join("metadata", "win32"), "directory of .w32meta.json files")
	if err := flags.Parse(args); err != nil {
		return err
	}
	registry, err := pipeline.LoadAll(*metadataDir)
	if err != nil {
		return err
	}

	var errorsFound, warnings []string
	addError := func(format string, args ...any) {
		errorsFound = append(errorsFound, fmt.Sprintf(format, args...))
	}
	addWarning := func(format string, args ...any) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	validEnumBases := map[string]bool{
		"int8": true, "uint8": true, "int16": true, "uint16": true,
		"int32": true, "uint32": true, "int64": true, "uint64": true,
	}

	for _, meta := range registry.Namespaces {
		namespace := meta.Namespace

		// Every ApiRef must resolve against the registry.
		pipeline.WalkNamespaceRefs(meta, func(ref *win32meta.TypeRef) {
			if ref.Kind != "ApiRef" || ref.Api == "" {
				return // nested anonymous refs resolve within their struct
			}
			if registry.ByNamespace[ref.Api] == nil {
				addError("[%s] reference to unknown namespace %s (%s)", namespace, ref.Api, ref.Name)
				return
			}
			key := ref.Api + "." + ref.Name
			resolved := false
			switch ref.TargetKind {
			case "Struct", "Union":
				_, resolved = registry.StructIndex[key]
			case "Enum":
				_, resolved = registry.EnumBaseIndex[key]
			case "Typedef":
				_, resolved = registry.TypedefIndex[key]
			case "FunctionPointer":
				_, resolved = registry.DelegateIndex[key]
			case "Com":
				_, resolved = registry.InterfaceIndex[key]
			default:
				addError("[%s] reference %s has no target kind", namespace, key)
				return
			}
			if !resolved {
				addError("[%s] dangling %s reference %s", namespace, ref.TargetKind, key)
			}
		})

		for i := range meta.Functions {
			if meta.Functions[i].DLL == "" {
				addError("[%s] function %s has no DLL import", namespace, meta.Functions[i].Name)
			}
		}
		for name, enum := range meta.Enums {
			if !validEnumBases[enum.BaseType] {
				addError("[%s] enum %s has invalid base type %q", namespace, name, enum.BaseType)
			}
			if len(enum.Members) == 0 {
				addWarning("[%s] enum %s has no members", namespace, name)
			}
		}
		for name, typedef := range meta.Typedefs {
			if typedef.Underlying.Kind == "" {
				addError("[%s] typedef %s has no underlying type", namespace, name)
			}
		}
		for name, comInterface := range meta.Interfaces {
			if comInterface.GUID == "" {
				addWarning("[%s] COM interface %s has no GUID", namespace, name)
			}
			if comInterface.BaseInterface != "" {
				key := comInterface.BaseInterfaceApi + "." + comInterface.BaseInterface
				if _, ok := registry.InterfaceIndex[key]; !ok {
					addError("[%s] COM interface %s has dangling base %s", namespace, name, key)
				}
			}
		}
	}

	sort.Strings(errorsFound)
	sort.Strings(warnings)
	for _, warning := range warnings {
		fmt.Printf("warning: %s\n", warning)
	}
	for _, message := range errorsFound {
		fmt.Printf("error: %s\n", message)
	}
	fmt.Printf("validate: %d namespaces, %d errors, %d warnings\n",
		len(registry.Namespaces), len(errorsFound), len(warnings))
	if len(errorsFound) > 0 {
		os.Exit(1)
	}
	return nil
}
