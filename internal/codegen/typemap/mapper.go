// Package typemap converts IR TypeRefs into Go types. It is the only place
// type decisions live: emitters consume Resolved values and never inspect
// TypeRefs themselves. Cross-namespace references populate the caller's
// ImportSet as a side effect, and every degradation to an untyped value is
// recorded in Diagnostics (the CI ratchet's input).
package typemap

import (
	"fmt"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/naming"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// ImportSet accumulates alias → import path pairs as resolution progresses.
type ImportSet map[string]string

// Kind classifies a resolved Go type for dispatch/marshaling decisions.
type Kind uint8

const (
	KindVoid          Kind = iota // no value (returns only)
	KindScalar                    // integer/float/bool value
	KindPointer                   // Go pointer (incl. unsafe.Pointer)
	KindHandleTypedef             // uintptr-backed named handle (HANDLE, HWND)
	KindPointerTypedef            // pointer-backed named typedef (PWSTR)
	KindScalarTypedef             // scalar-backed named typedef (BOOL, HRESULT)
	KindEnum                      // named enum type
	KindStruct                    // struct by value
	KindUnion                     // union by value
	KindArray                     // fixed array by value
	KindFuncPtr                   // callback (uintptr-backed named type)
	KindGUID                      // win32.GUID by value
	KindComPtr                    // COM interface pointer (*IFoo)
	KindUnsupported               // degraded; see Diagnostics
)

// Resolved is the pure-data result of one type resolution.
type Resolved struct {
	GoType string
	Kind   Kind
	// TypedefName/TypedefApi identify the named typedef when Kind is one of
	// the *Typedef kinds (used for failure-sentinel lookup).
	TypedefName string
	TypedefApi  string
}

// Context carries per-resolution state.
type Context struct {
	// Namespace is the short namespace being emitted ("System.Threading");
	// references into it stay unqualified.
	Namespace string
	// IsReturn marks return-position resolution.
	IsReturn bool
}

// Mapper resolves TypeRefs against the loaded Registry.
type Mapper struct {
	Registry   *pipeline.Registry
	ModulePath string

	// Blocked marks severed cross-namespace edges (import-cycle breaks):
	// Blocked[src][dst] forces references from src to dst to degrade to raw
	// types instead of importing.
	Blocked map[string]map[string]bool

	// SkippedTypes marks types ("Namespace.ExportedName") the generator
	// could not emit; references to them must degrade rather than name them.
	SkippedTypes map[string]bool

	// Diagnostics records every degradation (input to the CI ratchet).
	Diagnostics []string

	// Referenced records every namespace a resolution qualified against,
	// so the generator can emit the transitive closure.
	Referenced map[string]bool
}

// primitiveGoTypes maps IR Native names to Go types.
var primitiveGoTypes = map[string]string{
	"Boolean": "bool",
	"Char":    "uint16",
	"SByte":   "int8",
	"Byte":    "byte",
	"Int16":   "int16",
	"UInt16":  "uint16",
	"Int32":   "int32",
	"UInt32":  "uint32",
	"Int64":   "int64",
	"UInt64":  "uint64",
	"Single":  "float32",
	"Double":  "float64",
	"IntPtr":  "uintptr",
	"UIntPtr": "uintptr",
}

// GoType resolves one TypeRef. Cross-namespace references are qualified with
// the owning package alias and recorded in imports.
func (m *Mapper) GoType(ref *win32meta.TypeRef, ctx Context, imports ImportSet) Resolved {
	switch ref.Kind {
	case "Native":
		return m.resolveNative(ref, ctx)
	case "ApiRef":
		return m.resolveApiRef(ref, ctx, imports)
	case "PointerTo":
		return m.resolvePointer(ref, ctx, imports)
	case "Array":
		return m.resolveArray(ref, ctx, imports)
	}
	return m.degrade(ctx, "unknown TypeRef kind %q", ref.Kind)
}

func (m *Mapper) resolveNative(ref *win32meta.TypeRef, ctx Context) Resolved {
	switch ref.Name {
	case "Void":
		return Resolved{GoType: "", Kind: KindVoid}
	case "Guid":
		return Resolved{GoType: "win32.GUID", Kind: KindGUID}
	}
	if goType, ok := primitiveGoTypes[ref.Name]; ok {
		return Resolved{GoType: goType, Kind: KindScalar}
	}
	return m.degrade(ctx, "unmapped native type %q", ref.Name)
}

func (m *Mapper) resolveApiRef(ref *win32meta.TypeRef, ctx Context, imports ImportSet) Resolved {
	if ref.Api != ctx.Namespace && m.Blocked[ctx.Namespace][ref.Api] {
		return m.resolveBlockedRef(ref, ctx, imports)
	}
	if m.SkippedTypes[ref.Api+"."+naming.Export(ref.Name)] {
		return m.degrade(ctx, "reference to skipped type %s.%s", ref.Api, ref.Name)
	}
	goType := m.qualifiedName(ref.Api, ref.Name, ctx, imports)
	switch ref.TargetKind {
	case "Enum":
		return Resolved{GoType: goType, Kind: KindEnum}
	case "Struct":
		return Resolved{GoType: goType, Kind: KindStruct}
	case "Union":
		return Resolved{GoType: goType, Kind: KindUnion}
	case "FunctionPointer":
		return Resolved{GoType: goType, Kind: KindFuncPtr}
	case "Typedef":
		return m.resolveTypedefRef(ref, goType, ctx)
	case "Com":
		// A COM interface reference is an interface pointer.
		return Resolved{GoType: "*" + goType, Kind: KindComPtr}
	}
	return m.degrade(ctx, "ApiRef %s.%s with unknown target kind %q", ref.Api, ref.Name, ref.TargetKind)
}

// resolveBlockedRef degrades a reference across a severed edge to a raw
// shape that needs no import: enums flatten to their base type, typedefs to
// their underlying shape, function pointers to uintptr. By-value structs
// cannot degrade to a scalar; the gather layer blobs or skips them.
func (m *Mapper) resolveBlockedRef(ref *win32meta.TypeRef, ctx Context, imports ImportSet) Resolved {
	switch ref.TargetKind {
	case "Enum":
		if base := m.Registry.EnumBase(ref.Api, ref.Name); base != "" {
			m.note("[%s] cycle break: enum %s.%s flattened to %s", ctx.Namespace, ref.Api, ref.Name, base)
			return Resolved{GoType: base, Kind: KindScalar}
		}
	case "Typedef":
		if typedef := m.Registry.Typedef(ref.Api, ref.Name); typedef != nil {
			m.note("[%s] cycle break: typedef %s.%s flattened", ctx.Namespace, ref.Api, ref.Name)
			underlying := typedef.Underlying
			if underlying.Kind == "PointerTo" && underlying.Child != nil &&
				underlying.Child.Kind == "Native" && underlying.Child.Name == "Void" {
				return Resolved{GoType: "uintptr", Kind: KindHandleTypedef, TypedefName: ref.Name, TypedefApi: ref.Api}
			}
			return m.GoType(&underlying, ctx, imports)
		}
	case "FunctionPointer":
		m.note("[%s] cycle break: callback %s.%s flattened to uintptr", ctx.Namespace, ref.Api, ref.Name)
		return Resolved{GoType: "uintptr", Kind: KindFuncPtr}
	case "Com":
		m.note("[%s] cycle break: COM pointer %s.%s flattened to unsafe.Pointer", ctx.Namespace, ref.Api, ref.Name)
		return Resolved{GoType: "unsafe.Pointer", Kind: KindPointer}
	}
	return m.degrade(ctx, "cycle break: %s %s.%s not representable without import", ref.TargetKind, ref.Api, ref.Name)
}

// resolveTypedefRef classifies a named typedef by its underlying shape.
func (m *Mapper) resolveTypedefRef(ref *win32meta.TypeRef, goType string, ctx Context) Resolved {
	typedef := m.Registry.Typedef(ref.Api, ref.Name)
	if typedef == nil {
		return m.degrade(ctx, "unresolved typedef %s.%s", ref.Api, ref.Name)
	}
	resolved := Resolved{GoType: goType, TypedefName: ref.Name, TypedefApi: ref.Api}
	underlying := &typedef.Underlying
	switch {
	case underlying.Kind == "PointerTo" && underlying.Child != nil && underlying.Child.Kind == "Native" && underlying.Child.Name == "Void":
		// Handle-like: void* wrapped — uintptr-backed so the GC never sees
		// an OS handle as a Go pointer.
		resolved.Kind = KindHandleTypedef
	case underlying.Kind == "PointerTo":
		resolved.Kind = KindPointerTypedef
	default:
		resolved.Kind = KindScalarTypedef
	}
	return resolved
}

func (m *Mapper) resolvePointer(ref *win32meta.TypeRef, ctx Context, imports ImportSet) Resolved {
	child := ref.Child
	if child == nil {
		return m.degrade(ctx, "pointer with no pointee")
	}
	if child.Kind == "Native" && child.Name == "Void" {
		return Resolved{GoType: "unsafe.Pointer", Kind: KindPointer}
	}
	childCtx := ctx
	childCtx.IsReturn = false
	resolvedChild := m.GoType(child, childCtx, imports)
	if resolvedChild.Kind == KindUnsupported {
		// The pointee degraded (diagnostic already recorded); the pointer
		// itself is still perfectly representable.
		return Resolved{GoType: "unsafe.Pointer", Kind: KindPointer}
	}
	if resolvedChild.Kind == KindVoid {
		return Resolved{GoType: "unsafe.Pointer", Kind: KindPointer}
	}
	return Resolved{GoType: "*" + resolvedChild.GoType, Kind: KindPointer}
}

func (m *Mapper) resolveArray(ref *win32meta.TypeRef, ctx Context, imports ImportSet) Resolved {
	if ref.Child == nil {
		return m.degrade(ctx, "array with no element type")
	}
	element := m.GoType(ref.Child, ctx, imports)
	if element.Kind == KindUnsupported {
		return element
	}
	if ref.ArrayLen == 0 {
		return m.degrade(ctx, "array without fixed length")
	}
	return Resolved{
		GoType: fmt.Sprintf("[%d]%s", ref.ArrayLen, element.GoType),
		Kind:   KindArray,
	}
}

// qualifiedName renders Name qualified by the owning package (recording the
// import) unless it lives in the namespace being emitted.
func (m *Mapper) qualifiedName(api, name string, ctx Context, imports ImportSet) string {
	name = naming.Export(name)
	if api == ctx.Namespace || api == "" {
		return name
	}
	alias := naming.ImportAlias(api)
	imports[alias] = m.ModulePath + "/bindings/win32/" + naming.PackagePath(api)
	if m.Referenced == nil {
		m.Referenced = map[string]bool{}
	}
	m.Referenced[api] = true
	return alias + "." + name
}

// degrade records a diagnostic and returns the untyped fallback.
func (m *Mapper) degrade(ctx Context, format string, args ...any) Resolved {
	m.Diagnostics = append(m.Diagnostics,
		fmt.Sprintf("[%s] ", ctx.Namespace)+fmt.Sprintf(format, args...))
	return Resolved{GoType: "uintptr", Kind: KindUnsupported}
}

// note records a diagnostic without degrading.
func (m *Mapper) note(format string, args ...any) {
	m.Diagnostics = append(m.Diagnostics, fmt.Sprintf(format, args...))
}

// ── Syscall marshaling classification ─────────────────────────────────────────

// ArgClass says how a parameter converts to a SyscallN uintptr word.
type ArgClass uint8

const (
	ArgScalar      ArgClass = iota // uintptr(x)
	ArgPointer                     // uintptr(unsafe.Pointer(x))
	ArgUnsupported                 // cannot marshal (by-value struct, float…)
)

// ArgClassOf classifies a resolved parameter type for dispatch.
func ArgClassOf(resolved Resolved, goType string) ArgClass {
	switch resolved.Kind {
	case KindScalar:
		if goType == "float32" || goType == "float64" {
			// Floats travel in XMM registers, which syscall.SyscallN
			// cannot populate.
			return ArgUnsupported
		}
		if goType == "bool" {
			return ArgUnsupported // no direct uintptr conversion
		}
		return ArgScalar
	case KindEnum, KindHandleTypedef, KindScalarTypedef, KindFuncPtr, KindUnsupported:
		return ArgScalar
	case KindPointer, KindPointerTypedef, KindComPtr:
		return ArgPointer
	}
	return ArgUnsupported
}
