package idiowin

import (
	"fmt"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// ReExport is one raw top-level identifier the idiomatic tier may re-export.
// Kind is "type", "const", "var", or "func".
type ReExport struct {
	Name string
	Kind string
}

// reExportBodies holds the re-export source split by the file it belongs to,
// mirroring the raw tier's per-construct file layout.
type reExportBodies struct {
	typedefs  string // typedef aliases → <pkg>_typedefs.go
	enums     string // enum aliases → <pkg>_enums.go
	structs   string // struct aliases → <pkg>_structs.go
	delegates string // delegate aliases → <pkg>_delegates.go
	constants string // const/var aliases → <pkg>_constants.go
	functions string // pass-through function aliases → <pkg>_functions.go
	rawAlias  string // the raw package import alias these bodies reference
}

// empty reports whether no re-exports were produced.
func (b reExportBodies) empty() bool {
	return b.typedefs == "" && b.enums == "" && b.structs == "" &&
		b.delegates == "" && b.constants == "" && b.functions == ""
}

// buildReExports makes the idiomatic package self-contained: every raw
// top-level identifier the idiomatic tier does not itself define is
// re-exported (a type alias, or a const/var/func value alias) so consumers
// import only the idiomatic package, never bindings/win32.
//
// Identifiers the idiomatic tier already defines — wrapped functions, COM
// interface wrappers, handle closers, elevated returns — are skipped so the
// improved version wins. Because the aliases preserve type identity, a
// re-exported struct is still assignable to the raw calls the idiomatic
// wrappers make. The re-exports are grouped into the same files the raw
// tier uses (types / constants / functions).
func (g *Generator) buildReExports(meta *win32meta.NamespaceMeta) reExportBodies {
	symbols := g.reExports[meta.Namespace]
	if len(symbols) == 0 {
		return reExportBodies{}
	}
	sorted := append([]ReExport(nil), symbols...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	rawAlias := naming.ImportAlias(meta.Namespace)
	var typedefs, enums, structs, delegates, constants, functions strings.Builder
	for _, symbol := range sorted {
		// Already defined by the idiomatic package (a wrapper or improved
		// function) — the improved version wins.
		if g.claimedNames[symbol.Name] {
			continue
		}
		if !g.claimName(symbol.Name) {
			continue
		}
		qualified := rawAlias + "." + symbol.Name
		switch symbol.Kind {
		case "typedef":
			fmt.Fprintf(&typedefs, "type %s = %s\n", symbol.Name, qualified)
		case "enum":
			fmt.Fprintf(&enums, "type %s = %s\n", symbol.Name, qualified)
		case "struct":
			fmt.Fprintf(&structs, "type %s = %s\n", symbol.Name, qualified)
		case "delegate":
			fmt.Fprintf(&delegates, "type %s = %s\n", symbol.Name, qualified)
		case "type": // other named type (e.g. an unwrapped COM interface)
			fmt.Fprintf(&typedefs, "type %s = %s\n", symbol.Name, qualified)
		case "const":
			fmt.Fprintf(&constants, "const %s = %s\n", symbol.Name, qualified)
		case "var":
			fmt.Fprintf(&constants, "var %s = %s\n", symbol.Name, qualified)
		default: // "func" → a callable value alias, alongside the wrappers
			fmt.Fprintf(&functions, "var %s = %s\n", symbol.Name, qualified)
		}
	}
	return reExportBodies{
		typedefs:  typedefs.String(),
		enums:     enums.String(),
		structs:   structs.String(),
		delegates: delegates.String(),
		constants: constants.String(),
		functions: functions.String(),
		rawAlias:  rawAlias,
	}
}
