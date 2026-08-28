//go:build windows

package win32

import (
	"syscall"
	"testing"
	"unsafe"
)

// fakeObject is a COM object implemented in Go: a vtable of NewCallback
// trampolines whose QueryInterface hands back the object itself.
type fakeObject struct {
	IUnknown
	refs int32
}

// fakeStream is a stand-in for a generated interface struct: one vtable word.
type fakeStream struct{ IUnknown }

var (
	fakeQueryInterface = syscall.NewCallback(func(this *IUnknown, riid *GUID, ppv **IUnknown) uintptr {
		*ppv = this
		object := (*fakeObject)(unsafe.Pointer(this))
		object.refs++
		return uintptr(S_OK)
	})
	fakeAddRef = syscall.NewCallback(func(this *IUnknown) uintptr {
		object := (*fakeObject)(unsafe.Pointer(this))
		object.refs++
		return uintptr(object.refs)
	})
	fakeRelease = syscall.NewCallback(func(this *IUnknown) uintptr {
		object := (*fakeObject)(unsafe.Pointer(this))
		object.refs--
		return uintptr(object.refs)
	})
	fakeVtable = [3]uintptr{fakeQueryInterface, fakeAddRef, fakeRelease}
)

func newFakeObject() *fakeObject {
	return &fakeObject{
		IUnknown: IUnknown{LpVtbl: (*[1024]uintptr)(unsafe.Pointer(&fakeVtable))},
		refs:     1,
	}
}

func TestQueryInterfaceAs(t *testing.T) {
	object := newFakeObject()
	iid := GUID{Data1: 0x0c}
	stream, err := QueryInterface[fakeStream](&object.IUnknown, &iid)
	if err != nil {
		t.Fatalf("QueryInterface: %v", err)
	}
	if unsafe.Pointer(stream) != unsafe.Pointer(object) {
		t.Fatalf("QueryInterface returned %p, want the object %p", stream, object)
	}
	if object.refs != 2 {
		t.Fatalf("refs after QueryInterface = %d, want 2 (a new reference)", object.refs)
	}
	// The result is a usable interface struct: promoted Release works.
	if refs := stream.Release(); refs != 1 {
		t.Fatalf("Release returned %d, want 1", refs)
	}
}

func TestCast(t *testing.T) {
	object := newFakeObject()
	stream := Cast[fakeStream](&object.IUnknown)
	if unsafe.Pointer(stream) != unsafe.Pointer(object) {
		t.Fatalf("Cast returned %p, want %p", stream, object)
	}
	if object.refs != 1 {
		t.Fatalf("Cast must not AddRef: refs = %d", object.refs)
	}
	if got := stream.AddRef(); got != 2 {
		t.Fatalf("AddRef through the cast pointer = %d, want 2", got)
	}
}

func TestCastRejectsNonInterfaceTypes(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Cast to a two-word type did not panic")
		}
	}()
	Cast[[2]uintptr](newFakeObject().Cast())
}

// Cast is a tiny helper so the panic test reads naturally.
func (o *fakeObject) Cast() *IUnknown { return &o.IUnknown }
