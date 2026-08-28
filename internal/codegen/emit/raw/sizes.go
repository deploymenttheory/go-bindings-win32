package rawwin

import (
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// layout is a computed C size/alignment pair (amd64 model: pointers are 8;
// Windows arm64 shares every value) plus a census of the scalar leaves the
// composite flattens to, which decides the ARM64 homogeneous-floating-point-
// aggregate (HFA) rule: an aggregate of one to four float32s (or float64s)
// and nothing else travels in V registers, not X registers or memory.
type layout struct {
	size  uint32
	align uint32
	ok    bool
	// Leaf census (nested structs and arrays flattened): float32 leaves,
	// float64 leaves, and every other scalar/pointer leaf.
	f32, f64, other uint32
}

// hfa reports whether the layout is an HFA — one to four scalar leaves, all
// float32 or all float64 — and the element type.
func (l layout) hfa() (count uint32, isFloat64 bool, ok bool) {
	if !l.ok || l.other != 0 {
		return 0, false, false
	}
	switch {
	case l.f32 > 0 && l.f64 == 0 && l.f32 <= 4:
		return l.f32, false, true
	case l.f64 > 0 && l.f32 == 0 && l.f64 <= 4:
		return l.f64, true, true
	}
	return 0, false, false
}

// registerSized reports whether the size is one the x64 convention passes in
// (and returns from) an integer register as if it were an integer.
func (l layout) registerSized() bool {
	switch l.size {
	case 1, 2, 4, 8:
		return l.ok
	}
	return false
}

// nativeLayouts gives size/alignment (and leaf class) for IR Native primitives.
var nativeLayouts = map[string]layout{
	"Boolean": {size: 1, align: 1, ok: true, other: 1},
	"SByte":   {size: 1, align: 1, ok: true, other: 1},
	"Byte":    {size: 1, align: 1, ok: true, other: 1},
	"Char":    {size: 2, align: 2, ok: true, other: 1},
	"Int16":   {size: 2, align: 2, ok: true, other: 1},
	"UInt16":  {size: 2, align: 2, ok: true, other: 1},
	"Int32":   {size: 4, align: 4, ok: true, other: 1},
	"UInt32":  {size: 4, align: 4, ok: true, other: 1},
	"Single":  {size: 4, align: 4, ok: true, f32: 1},
	"Int64":   {size: 8, align: 8, ok: true, other: 1},
	"UInt64":  {size: 8, align: 8, ok: true, other: 1},
	"Double":  {size: 8, align: 8, ok: true, f64: 1},
	"IntPtr":  {size: 8, align: 8, ok: true, other: 1},
	"UIntPtr": {size: 8, align: 8, ok: true, other: 1},
	"Guid":    {size: 16, align: 4, ok: true, other: 4},
}

// wordLayout is the layout of one pointer-sized non-float leaf.
var wordLayout = layout{size: 8, align: 8, ok: true, other: 1}

// layoutOf computes the C layout of a type reference. nested resolves
// same-struct anonymous types. Returns ok=false when a layout cannot be
// derived (the caller records a diagnostic and skips).
func (g *Generator) layoutOf(ref *win32meta.TypeRef, nested map[string]win32meta.Struct) layout {
	switch ref.Kind {
	case "Native":
		if l, ok := nativeLayouts[ref.Name]; ok {
			return l
		}
		return layout{}
	case "PointerTo":
		return wordLayout
	case "Array":
		element := g.layoutOf(ref.Child, nested)
		if !element.ok || ref.ArrayLen == 0 {
			return layout{}
		}
		return layout{
			size: element.size * ref.ArrayLen, align: element.align, ok: true,
			f32: element.f32 * ref.ArrayLen, f64: element.f64 * ref.ArrayLen, other: element.other * ref.ArrayLen,
		}
	case "ApiRef":
		return g.layoutOfApiRef(ref, nested)
	}
	return layout{}
}

func (g *Generator) layoutOfApiRef(ref *win32meta.TypeRef, nested map[string]win32meta.Struct) layout {
	// Anonymous nested types live on the enclosing struct, not the registry.
	if ref.Api == "" {
		if nested != nil {
			if definition, ok := nested[ref.Name]; ok {
				return g.layoutOfStruct(&definition)
			}
		}
		return layout{}
	}
	switch ref.TargetKind {
	case "Enum":
		if base := g.registry.EnumBase(ref.Api, ref.Name); base != "" {
			return nativeLayouts[goBaseToNative(base)]
		}
		return nativeLayouts["Int32"]
	case "Typedef":
		if typedef := g.registry.Typedef(ref.Api, ref.Name); typedef != nil {
			return g.layoutOf(&typedef.Underlying, nil)
		}
		return layout{}
	case "FunctionPointer", "Com":
		return wordLayout
	case "Struct", "Union":
		if definition := g.registry.StructIndex[ref.Api+"."+ref.Name]; definition != nil {
			// Layouts must describe the variant the generator emits.
			if chosen := pickAmd64Variant(definition); chosen != nil {
				return g.layoutOfStruct(chosen)
			}
		}
		return layout{}
	}
	return layout{}
}

// structLayout is a struct's computed C layout with per-field offsets.
type structLayout struct {
	size    uint32
	align   uint32
	offsets []uint32
	ok      bool
	// Leaf census, as in layout.
	f32, f64, other uint32
}

// layoutOfStruct computes a struct or union's C layout, honoring its
// PackingSize when it narrows alignment.
func (g *Generator) layoutOfStruct(definition *win32meta.Struct) layout {
	detailed := g.structLayoutOf(definition, true)
	return layout{size: detailed.size, align: detailed.align, ok: detailed.ok, f32: detailed.f32, f64: detailed.f64, other: detailed.other}
}

// structLayoutOf computes size, alignment, and field offsets. clampPacking
// applies the struct's own PackingSize (the C layout); passing false yields
// the layout Go's natural alignment produces for the emitted fields —
// comparing the two decides whether a packed struct is representable.
//
// A union's leaf census sums every member's leaves, so a union mixing floats
// with anything else (VARIANT) is never classified as an HFA.
func (g *Generator) structLayoutOf(definition *win32meta.Struct, clampPacking bool) structLayout {
	var result structLayout
	result.offsets = make([]uint32, 0, len(definition.Fields))
	for i := range definition.Fields {
		field := g.layoutOf(&definition.Fields[i].Type, definition.NestedTypes)
		if !field.ok {
			return structLayout{}
		}
		fieldAlign := field.align
		if clampPacking && definition.PackingSize != 0 && uint32(definition.PackingSize) < fieldAlign {
			fieldAlign = uint32(definition.PackingSize)
		}
		if fieldAlign > result.align {
			result.align = fieldAlign
		}
		if definition.IsUnion {
			result.offsets = append(result.offsets, 0)
			if field.size > result.size {
				result.size = field.size
			}
		} else {
			result.size = roundUp(result.size, fieldAlign)
			result.offsets = append(result.offsets, result.size)
			result.size += field.size
		}
		result.f32 += field.f32
		result.f64 += field.f64
		result.other += field.other
	}
	if result.align == 0 {
		result.align = 1
	}
	result.size = roundUp(result.size, result.align)
	result.ok = true
	return result
}

func roundUp(value, multiple uint32) uint32 {
	if multiple == 0 {
		return value
	}
	return (value + multiple - 1) / multiple * multiple
}

// goBaseToNative converts an enum's Go base name back to the Native
// vocabulary for layout lookup.
func goBaseToNative(goBase string) string {
	switch goBase {
	case "int8":
		return "SByte"
	case "uint8":
		return "Byte"
	case "int16":
		return "Int16"
	case "uint16":
		return "UInt16"
	case "int32":
		return "Int32"
	case "uint32":
		return "UInt32"
	case "int64":
		return "Int64"
	case "uint64":
		return "UInt64"
	}
	return "UInt32"
}
