package rawwin

import (
	"testing"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

const testModulePath = "github.com/deploymenttheory/go-bindings-win32"

func guidPtrType() win32meta.TypeRef {
	return win32meta.TypeRef{Kind: "PointerTo", Child: &win32meta.TypeRef{Kind: "Native", Name: "Guid"}}
}

func voidPtrType() win32meta.TypeRef {
	return win32meta.TypeRef{Kind: "PointerTo", Child: &win32meta.TypeRef{Kind: "Native", Name: "Void"}}
}

func voidDoublePtrType() win32meta.TypeRef {
	inner := voidPtrType()
	return win32meta.TypeRef{Kind: "PointerTo", Child: &inner}
}

// param builds a Param with the ingest defaults for the index sentinels.
func param(name string, typ win32meta.TypeRef, mutate ...func(*win32meta.Param)) win32meta.Param {
	p := win32meta.Param{
		Name:                       name,
		Type:                       typ,
		NativeArrayCountParamIndex: -1,
		MemorySizeBytesParamIndex:  -1,
		IidParamIndex:              -1,
	}
	for _, m := range mutate {
		m(&p)
	}
	return p
}

func in(p *win32meta.Param)       { p.IsIn = true }
func out(p *win32meta.Param)      { p.IsOut = true }
func reserved(p *win32meta.Param) { p.IsReserved = true }
func comOutPtr(p *win32meta.Param) {
	p.IsOut = true
	p.IsComOutPtr = true
}

// retype runs retypeComOutParams over params with placeholder resolutions and
// returns the resulting Go types plus the recorded imports.
func retype(params []win32meta.Param) ([]string, typemap.ImportSet) {
	resolved := make([]typemap.Resolved, len(params))
	for i := range params {
		resolved[i] = typemap.Resolved{GoType: "*unsafe.Pointer", Kind: typemap.KindPointer}
	}
	imports := typemap.ImportSet{}
	retypeComOutParams(params, resolved, imports, testModulePath)
	types := make([]string, len(params))
	for i := range resolved {
		types[i] = resolved[i].GoType
	}
	return types, imports
}

const iunknown = "**win32.IUnknown"

func TestRetypeComOutParams(t *testing.T) {
	t.Run("attributed ComOutPtr still retypes", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("ppv", voidDoublePtrType(), comOutPtr),
		})
		if types[0] != iunknown {
			t.Errorf("ppv = %q, want %q", types[0], iunknown)
		}
	})

	t.Run("adjacent riid pair retypes", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("riid", guidPtrType(), in),
			param("ppvObject", voidDoublePtrType(), out),
			param("pMalloc", voidPtrType(), in),
		})
		if types[1] != iunknown {
			t.Errorf("ppvObject = %q, want %q", types[1], iunknown)
		}
	})

	t.Run("unique pair with iid after the out retypes", func(t *testing.T) {
		// DirectDrawCreateEx shape: the void** precedes the iid param.
		types, _ := retype([]win32meta.Param{
			param("lpGuid", guidPtrType(), in),
			param("lplpDD", voidDoublePtrType(), out),
			param("iid", guidPtrType(), in),
			param("pUnkOuter", voidPtrType(), in),
		})
		if types[1] != iunknown {
			t.Errorf("lplpDD = %q, want %q", types[1], iunknown)
		}
	})

	t.Run("guid param without iid in the name does not pair", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("guidPropSet", guidPtrType(), in),
			param("ppvData", voidDoublePtrType(), out),
		})
		if types[1] != "*unsafe.Pointer" {
			t.Errorf("ppvData = %q, want untouched", types[1])
		}
	})

	t.Run("out riid does not pair", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("riid", guidPtrType(), in, out),
			param("ppv", voidDoublePtrType(), out),
		})
		if types[1] != "*unsafe.Pointer" {
			t.Errorf("ppv = %q, want untouched", types[1])
		}
	})

	t.Run("reserved riid does not pair", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("riid", guidPtrType(), in, reserved),
			param("ppv", voidDoublePtrType(), out),
		})
		if types[1] != "*unsafe.Pointer" {
			t.Errorf("ppv = %q, want untouched", types[1])
		}
	})

	t.Run("two non-adjacent candidates stay ambiguous", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("riid", guidPtrType(), in),
			param("cb", win32meta.TypeRef{Kind: "Native", Name: "UInt32"}, in),
			param("ppvA", voidDoublePtrType(), out),
			param("ppvB", voidDoublePtrType(), out),
		})
		if types[2] != "*unsafe.Pointer" || types[3] != "*unsafe.Pointer" {
			t.Errorf("ppvA/ppvB = %q/%q, want both untouched", types[2], types[3])
		}
	})

	t.Run("attributed sibling keeps the sole riid to itself", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("riid", guidPtrType(), in),
			param("pUnkOuter", voidPtrType(), in),
			param("ppProxy", voidDoublePtrType(), comOutPtr),
			param("ppv", voidDoublePtrType(), out),
		})
		if types[2] != iunknown {
			t.Errorf("ppProxy = %q, want %q", types[2], iunknown)
		}
		if types[3] != "*unsafe.Pointer" {
			t.Errorf("ppv = %q, want untouched (uniqueness broken by attributed sibling)", types[3])
		}
	})

	t.Run("single-level void pointer is untouched", func(t *testing.T) {
		types, _ := retype([]win32meta.Param{
			param("riid", guidPtrType(), in),
			param("pvData", voidPtrType(), out),
		})
		if types[1] != "*unsafe.Pointer" {
			t.Errorf("pvData = %q, want untouched", types[1])
		}
	})

	t.Run("heuristic hit records the win32 import", func(t *testing.T) {
		_, imports := retype([]win32meta.Param{
			param("riid", guidPtrType(), in),
			param("ppv", voidDoublePtrType(), out),
		})
		if imports["win32"] != testModulePath+"/bindings/runtime/win32" {
			t.Errorf("win32 import = %q, want runtime path", imports["win32"])
		}
	})
}
