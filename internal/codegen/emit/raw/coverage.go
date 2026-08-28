package rawwin

import "sort"

// NamespaceCoverage counts, for one emitted namespace, how many of the
// metadata's constructs the generator emitted. Totals count what the
// generator considered (functions after amd64 filtering and same-name
// deduplication; every COM method; every top-level struct; every [RAIIFree]
// typedef), so emitted/total is the honest coverage ratio the coverage
// report publishes.
type NamespaceCoverage struct {
	Namespace                   string
	Functions, FunctionsEmitted int
	Methods, MethodsEmitted     int
	Structs, StructsEmitted     int
	Closers, ClosersEmitted     int
}

// Coverage returns the per-namespace counts of the last EmitAll, sorted by
// namespace.
func (g *Generator) Coverage() []NamespaceCoverage {
	names := make([]string, 0, len(g.coverage))
	for name := range g.coverage {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]NamespaceCoverage, 0, len(names))
	for _, name := range names {
		result = append(result, *g.coverage[name])
	}
	return result
}

// coverageOf returns the namespace's counter, creating it on first use.
func (g *Generator) coverageOf(namespace string) *NamespaceCoverage {
	if g.coverage == nil {
		g.coverage = map[string]*NamespaceCoverage{}
	}
	entry := g.coverage[namespace]
	if entry == nil {
		entry = &NamespaceCoverage{Namespace: namespace}
		g.coverage[namespace] = entry
	}
	return entry
}
