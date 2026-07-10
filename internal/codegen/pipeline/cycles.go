package pipeline

import (
	"sort"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// ComputeBlockedImports builds the cross-namespace reference graph, detects
// import cycles, and returns the edge set to sever: blocked[src][dst] means
// references from src to dst must degrade to raw types instead of importing.
// Edges are broken lowest-reference-count first (fewest degradations),
// deterministically — except base-interface embedding edges, which carry a
// large weight bonus because severing one demotes a whole interface to a
// rootless vtable (losing its inherited method set).
func ComputeBlockedImports(registry *Registry) map[string]map[string]bool {
	const baseEmbedWeight = 1000

	// Weighted edges: references from one namespace to another.
	edges := map[string]map[string]int{}
	for _, meta := range registry.Namespaces {
		weights := map[string]int{}
		count := func(ref *win32meta.TypeRef) {
			if ref.Kind == "ApiRef" && ref.Api != "" && ref.Api != meta.Namespace {
				weights[ref.Api]++
			}
		}
		WalkNamespaceRefs(meta, count)
		for name := range meta.Interfaces {
			comInterface := meta.Interfaces[name]
			if comInterface.BaseInterfaceApi != "" && comInterface.BaseInterfaceApi != meta.Namespace {
				weights[comInterface.BaseInterfaceApi] += baseEmbedWeight
			}
		}
		if len(weights) > 0 {
			edges[meta.Namespace] = weights
		}
	}

	blocked := map[string]map[string]bool{}
	for {
		cycle := findCycle(edges)
		if cycle == nil {
			return blocked
		}
		src, dst := lightestEdge(edges, cycle)
		delete(edges[src], dst)
		if blocked[src] == nil {
			blocked[src] = map[string]bool{}
		}
		blocked[src][dst] = true
	}
}

// WalkNamespaceRefs visits every TypeRef in a namespace (functions, structs
// incl. nested and arch variants, typedefs, delegates, constants, and COM
// interface method signatures).
func WalkNamespaceRefs(meta *win32meta.NamespaceMeta, visit func(*win32meta.TypeRef)) {
	var walkRef func(*win32meta.TypeRef)
	walkRef = func(ref *win32meta.TypeRef) {
		if ref == nil {
			return
		}
		visit(ref)
		walkRef(ref.Child)
	}
	var walkStruct func(*win32meta.Struct)
	walkStruct = func(definition *win32meta.Struct) {
		for i := range definition.Fields {
			walkRef(&definition.Fields[i].Type)
		}
		for name := range definition.NestedTypes {
			nested := definition.NestedTypes[name]
			walkStruct(&nested)
		}
		for i := range definition.ArchVariants {
			walkStruct(&definition.ArchVariants[i])
		}
	}

	for i := range meta.Functions {
		function := &meta.Functions[i]
		walkRef(&function.Return)
		for j := range function.Params {
			walkRef(&function.Params[j].Type)
		}
	}
	for name := range meta.Structs {
		definition := meta.Structs[name]
		walkStruct(&definition)
	}
	for name := range meta.Typedefs {
		typedef := meta.Typedefs[name]
		walkRef(&typedef.Underlying)
	}
	for name := range meta.Delegates {
		delegate := meta.Delegates[name]
		walkRef(&delegate.Return)
		for j := range delegate.Params {
			walkRef(&delegate.Params[j].Type)
		}
	}
	for i := range meta.Constants {
		walkRef(&meta.Constants[i].Type)
	}
	for name := range meta.Interfaces {
		comInterface := meta.Interfaces[name]
		for i := range comInterface.Methods {
			method := &comInterface.Methods[i]
			walkRef(&method.Return)
			for j := range method.Params {
				walkRef(&method.Params[j].Type)
			}
		}
	}
}

// findCycle DFSes the edge graph and returns one cycle as a node path
// (v0 → v1 → … → v0), or nil when the graph is acyclic. Iteration order is
// sorted for determinism.
func findCycle(edges map[string]map[string]int) []string {
	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)
	state := map[string]int{}
	var stack []string
	var cycle []string

	nodes := make([]string, 0, len(edges))
	for node := range edges {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	var visit func(node string) bool
	visit = func(node string) bool {
		state[node] = inStack
		stack = append(stack, node)
		targets := make([]string, 0, len(edges[node]))
		for target := range edges[node] {
			targets = append(targets, target)
		}
		sort.Strings(targets)
		for _, target := range targets {
			switch state[target] {
			case inStack:
				// Slice the cycle out of the stack.
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == target {
						cycle = append([]string{}, stack[i:]...)
						cycle = append(cycle, target)
						return true
					}
				}
			case unvisited:
				if visit(target) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[node] = done
		return false
	}
	for _, node := range nodes {
		if state[node] == unvisited && visit(node) {
			return cycle
		}
	}
	return nil
}

// lightestEdge picks the cycle edge with the fewest references (cheapest to
// degrade), breaking ties by name for determinism.
func lightestEdge(edges map[string]map[string]int, cycle []string) (string, string) {
	bestSrc, bestDst := cycle[0], cycle[1]
	bestWeight := edges[bestSrc][bestDst]
	for i := 0; i < len(cycle)-1; i++ {
		src, dst := cycle[i], cycle[i+1]
		weight := edges[src][dst]
		if weight < bestWeight || (weight == bestWeight && src+"→"+dst < bestSrc+"→"+bestDst) {
			bestSrc, bestDst, bestWeight = src, dst, weight
		}
	}
	return bestSrc, bestDst
}
