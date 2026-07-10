// Package pipeline loads the committed .w32meta.json IR into a Registry and
// drives the emitters.
package pipeline

import (
	"fmt"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Registry is the cross-namespace resolution index. Unlike the macOS
// generator there is no ownership heuristic: the winmd namespaces are
// authoritative, so every index is a straight projection of the IR.
type Registry struct {
	Namespaces []*win32meta.NamespaceMeta

	// ByNamespace maps the short namespace ("System.Threading") to its meta.
	ByNamespace map[string]*win32meta.NamespaceMeta

	// TypedefIndex maps "Api.Name" → the typedef definition.
	TypedefIndex map[string]*win32meta.Typedef
	// EnumBaseIndex maps "Api.Name" → the enum's Go base type.
	EnumBaseIndex map[string]string
	// StructIndex maps "Api.Name" → the struct definition.
	StructIndex map[string]*win32meta.Struct
	// DelegateIndex maps "Api.Name" → the function-pointer definition.
	DelegateIndex map[string]*win32meta.FuncPointer
	// InterfaceIndex maps "Api.Name" → the COM interface definition.
	InterfaceIndex map[string]*win32meta.ComInterface
}

// qualified builds the "Api.Name" index key.
func qualified(api, name string) string { return api + "." + name }

// LoadAll reads every namespace metadata file in dir into a Registry.
func LoadAll(dir string) (*Registry, error) {
	namespaces, err := win32meta.ReadAll(dir)
	if err != nil {
		return nil, err
	}
	if len(namespaces) == 0 {
		return nil, fmt.Errorf("pipeline: no .w32meta.json files in %s (run 'generate ingest')", dir)
	}
	registry := &Registry{
		Namespaces:     namespaces,
		ByNamespace:    make(map[string]*win32meta.NamespaceMeta, len(namespaces)),
		TypedefIndex:   map[string]*win32meta.Typedef{},
		EnumBaseIndex:  map[string]string{},
		StructIndex:    map[string]*win32meta.Struct{},
		DelegateIndex:  map[string]*win32meta.FuncPointer{},
		InterfaceIndex: map[string]*win32meta.ComInterface{},
	}
	for _, meta := range namespaces {
		registry.ByNamespace[meta.Namespace] = meta
		for name := range meta.Typedefs {
			typedef := meta.Typedefs[name]
			registry.TypedefIndex[qualified(meta.Namespace, name)] = &typedef
		}
		for name := range meta.Enums {
			registry.EnumBaseIndex[qualified(meta.Namespace, name)] = meta.Enums[name].BaseType
		}
		for name := range meta.Structs {
			definition := meta.Structs[name]
			registry.StructIndex[qualified(meta.Namespace, name)] = &definition
		}
		for name := range meta.Delegates {
			delegate := meta.Delegates[name]
			registry.DelegateIndex[qualified(meta.Namespace, name)] = &delegate
		}
		for name := range meta.Interfaces {
			comInterface := meta.Interfaces[name]
			registry.InterfaceIndex[qualified(meta.Namespace, name)] = &comInterface
		}
	}
	return registry, nil
}

// Typedef resolves a typedef reference, or nil.
func (r *Registry) Typedef(api, name string) *win32meta.Typedef {
	return r.TypedefIndex[qualified(api, name)]
}

// Interface resolves a COM interface reference, or nil.
func (r *Registry) Interface(api, name string) *win32meta.ComInterface {
	return r.InterfaceIndex[qualified(api, name)]
}

// VtableStartSlot returns the vtable slot index where the named interface's
// OWN methods begin: the total method count of its base-interface chain.
// ok is false on a dangling or cyclic base chain.
func (r *Registry) VtableStartSlot(api, name string) (int, bool) {
	slot := 0
	seen := map[string]bool{}
	current := r.Interface(api, name)
	for current != nil && current.BaseInterface != "" {
		key := qualified(current.BaseInterfaceApi, current.BaseInterface)
		if seen[key] {
			return 0, false // cyclic inheritance: malformed metadata
		}
		seen[key] = true
		base := r.InterfaceIndex[key]
		if base == nil {
			return 0, false
		}
		slot += len(base.Methods)
		current = base
	}
	return slot, true
}

// EnumBase resolves an enum's Go base type, or "".
func (r *Registry) EnumBase(api, name string) string {
	return r.EnumBaseIndex[qualified(api, name)]
}
