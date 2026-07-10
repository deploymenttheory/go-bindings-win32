package rawwin

import (
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// layout is a computed C size/alignment pair (amd64 model: pointers are 8).
type layout struct {
	size  uint32
	align uint32
	ok    bool
}

// nativeLayouts gives size/alignment for IR Native primitives on amd64.
var nativeLayouts = map[string]layout{
	"Boolean": {1, 1, true},
	"SByte":   {1, 1, true},
	"Byte":    {1, 1, true},
	"Char":    {2, 2, true},
	"Int16":   {2, 2, true},
	"UInt16":  {2, 2, true},
	"Int32":   {4, 4, true},
	"UInt32":  {4, 4, true},
	"Single":  {4, 4, true},
	"Int64":   {8, 8, true},
	"UInt64":  {8, 8, true},
	"Double":  {8, 8, true},
	"IntPtr":  {8, 8, true},
	"UIntPtr": {8, 8, true},
	"Guid":    {16, 4, true},
}

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
		return layout{8, 8, true}
	case "Array":
		element := g.layoutOf(ref.Child, nested)
		if !element.ok || ref.ArrayLen == 0 {
			return layout{}
		}
		return layout{element.size * ref.ArrayLen, element.align, true}
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
		return layout{4, 4, true}
	case "Typedef":
		if typedef := g.registry.Typedef(ref.Api, ref.Name); typedef != nil {
			return g.layoutOf(&typedef.Underlying, nil)
		}
		return layout{}
	case "FunctionPointer", "Com":
		return layout{8, 8, true}
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
}

// layoutOfStruct computes a struct or union's C layout, honoring its
// PackingSize when it narrows alignment.
func (g *Generator) layoutOfStruct(definition *win32meta.Struct) layout {
	detailed := g.structLayoutOf(definition, true)
	return layout{detailed.size, detailed.align, detailed.ok}
}

// structLayoutOf computes size, alignment, and field offsets. clampPacking
// applies the struct's own PackingSize (the C layout); passing false yields
// the layout Go's natural alignment produces for the emitted fields —
// comparing the two decides whether a packed struct is representable.
func (g *Generator) structLayoutOf(definition *win32meta.Struct, clampPacking bool) structLayout {
	var size, align uint32
	offsets := make([]uint32, 0, len(definition.Fields))
	for i := range definition.Fields {
		field := g.layoutOf(&definition.Fields[i].Type, definition.NestedTypes)
		if !field.ok {
			return structLayout{}
		}
		fieldAlign := field.align
		if clampPacking && definition.PackingSize != 0 && uint32(definition.PackingSize) < fieldAlign {
			fieldAlign = uint32(definition.PackingSize)
		}
		if fieldAlign > align {
			align = fieldAlign
		}
		if definition.IsUnion {
			offsets = append(offsets, 0)
			if field.size > size {
				size = field.size
			}
		} else {
			size = roundUp(size, fieldAlign)
			offsets = append(offsets, size)
			size += field.size
		}
	}
	if align == 0 {
		align = 1
	}
	return structLayout{roundUp(size, align), align, offsets, true}
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
