package rawwin

import (
	"sort"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Architecture-specific struct layouts.
//
// Windows amd64 and arm64 share the LLP64 data model, so almost every struct
// has one layout for both. A handful (CONTEXT, KNONVOLATILE_CONTEXT_POINTERS,
// SLIST_HEADER, the unwind and minidump thread records) are genuinely
// different per architecture, and any struct embedding one of those by value
// inherits the difference. Those are emitted twice — into
// <pkg>_structs_amd64.go and <pkg>_structs_arm64.go under per-architecture
// build tags — from their own metadata variant, and their ABI assertions are
// recorded per architecture so the arm64 test job asserts the arm64 layout.

// emitArches are the architectures the bindings target, in emission order.
var emitArches = []string{"amd64", "arm64"}

// pickVariant returns the struct layout for arch: a variant listing the
// architecture, else (arm64 only) the amd64 layout, which arm64 shares
// whenever the metadata records no arm64-specific one. nil when the struct
// exists on neither.
func pickVariant(definition *win32meta.Struct, arch string) *win32meta.Struct {
	matches := func(s *win32meta.Struct, want string) bool {
		if len(s.Availability.Architectures) == 0 {
			return true
		}
		for _, candidate := range s.Availability.Architectures {
			if candidate == want {
				return true
			}
		}
		return false
	}
	find := func(want string) *win32meta.Struct {
		if matches(definition, want) {
			return definition
		}
		for i := range definition.ArchVariants {
			if matches(&definition.ArchVariants[i], want) {
				return &definition.ArchVariants[i]
			}
		}
		return nil
	}
	if chosen := find(arch); chosen != nil {
		return chosen
	}
	if arch == "arm64" {
		return find("amd64")
	}
	return nil
}

// pickAmd64Variant returns the amd64-compatible layout of a struct, or nil.
// Pure — usable from layout computation without emitting diagnostics.
func pickAmd64Variant(definition *win32meta.Struct) *win32meta.Struct {
	return pickVariant(definition, "amd64")
}

// computeArchStructs finds every struct whose amd64 and arm64 layouts differ
// — directly, or because a by-value field (through nested types and arrays,
// never pointers) references one that does — keyed "Namespace.Name".
func (g *Generator) computeArchStructs() {
	g.archStructs = map[string]bool{}
	for key, definition := range g.registry.StructIndex {
		amd64, arm64 := pickVariant(definition, "amd64"), pickVariant(definition, "arm64")
		if amd64 != nil && arm64 != nil && amd64 != arm64 {
			g.archStructs[key] = true
		}
	}
	// By-value containers inherit the divergence; iterate to a fixed point.
	for {
		grew := false
		for key, definition := range g.registry.StructIndex {
			if g.archStructs[key] {
				continue
			}
			for _, arch := range emitArches {
				if chosen := pickVariant(definition, arch); chosen != nil && g.embedsArchStruct(chosen) {
					g.archStructs[key] = true
					grew = true
					break
				}
			}
		}
		if !grew {
			return
		}
	}
}

// embedsArchStruct reports whether a struct layout contains an
// architecture-specific struct by value.
func (g *Generator) embedsArchStruct(definition *win32meta.Struct) bool {
	var byValue func(ref *win32meta.TypeRef) bool
	byValue = func(ref *win32meta.TypeRef) bool {
		switch ref.Kind {
		case "Array":
			return ref.Child != nil && byValue(ref.Child)
		case "ApiRef":
			if ref.Api == "" {
				if nested, ok := definition.NestedTypes[ref.Name]; ok {
					return g.embedsArchStruct(&nested)
				}
				return false
			}
			return (ref.TargetKind == "Struct" || ref.TargetKind == "Union") && g.archStructs[ref.Api+"."+ref.Name]
		}
		return false
	}
	for i := range definition.Fields {
		if byValue(&definition.Fields[i].Type) {
			return true
		}
	}
	return false
}

// isArchStruct reports whether the namespace's struct is emitted per
// architecture.
func (g *Generator) isArchStruct(namespace, name string) bool {
	return g.archStructs[namespace+"."+name]
}

// buildArchStructModels emits the namespace's architecture-specific structs
// for every architecture, from each one's variant, with the layout engine
// (and the ABI records) switched to it. Every pass starts from the same
// name-claim state — the two files declare the same type names, and each
// may introduce nested types the other lacks — and the union of the claims
// is kept afterwards so later value names cannot collide with any of them.
// The result maps architecture → models (the map is empty when the
// namespace has no such structs).
func (g *Generator) buildArchStructModels(meta *win32meta.NamespaceMeta, imports typemap.ImportSet) map[string][]view.StructModel {
	names := make([]string, 0, len(meta.Structs))
	for name := range meta.Structs {
		if g.isArchStruct(meta.Namespace, name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)

	baseClaims, baseTypes := copySet(g.claimedNames), copySet(g.typeNames)
	unionClaims, unionTypes := copySet(g.claimedNames), copySet(g.typeNames)
	previousArch := g.layoutArch
	defer func() { g.layoutArch = previousArch }()

	result := map[string][]view.StructModel{}
	consumedTypes := map[string]bool{}
	for _, arch := range emitArches {
		g.claimedNames, g.typeNames = copySet(baseClaims), copySet(baseTypes)
		g.layoutArch = arch
		var models []view.StructModel
		for _, name := range names {
			definition := meta.Structs[name]
			chosen := pickVariant(&definition, arch)
			if chosen == nil {
				g.diag("struct %s: no %s layout, skipped on %s", name, arch, arch)
				continue
			}
			models = append(models, g.buildStructTree(meta, naming.Export(name), chosen, imports, false)...)
		}
		result[arch] = models
		for name := range g.claimedNames {
			unionClaims[name] = true
		}
		for name := range baseTypes {
			if !g.typeNames[name] {
				consumedTypes[name] = true // this pass emitted the type
			}
		}
	}
	// A type name consumed by either pass is consumed for good.
	for name := range consumedTypes {
		delete(unionTypes, name)
	}
	g.claimedNames, g.typeNames = unionClaims, unionTypes
	return result
}

func copySet(set map[string]bool) map[string]bool {
	copied := make(map[string]bool, len(set))
	for key, value := range set {
		copied[key] = value
	}
	return copied
}
