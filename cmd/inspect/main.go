// Command inspect dumps a .w32meta.json namespace file: a summary by
// default, or one construct in full with --name.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

func main() {
	name := flag.String("name", "", "dump one construct (function/struct/enum/interface/typedef/delegate) as JSON")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: inspect [--name <TypeOrFunction>] <path-to.w32meta.json>")
		os.Exit(2)
	}
	meta, err := win32meta.Read(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "inspect:", err)
		os.Exit(1)
	}
	if *name != "" {
		dumpOne(meta, *name)
		return
	}
	summarize(meta)
}

func dumpOne(meta *win32meta.NamespaceMeta, name string) {
	var found any
	if s, ok := meta.Structs[name]; ok {
		found = s
	} else if e, ok := meta.Enums[name]; ok {
		found = e
	} else if i, ok := meta.Interfaces[name]; ok {
		found = i
	} else if d, ok := meta.Delegates[name]; ok {
		found = d
	} else if t, ok := meta.Typedefs[name]; ok {
		found = t
	} else {
		for i := range meta.Functions {
			if meta.Functions[i].Name == name {
				found = meta.Functions[i]
				break
			}
		}
		for i := range meta.Constants {
			if meta.Constants[i].Name == name {
				found = meta.Constants[i]
				break
			}
		}
	}
	if found == nil {
		fmt.Fprintf(os.Stderr, "inspect: %q not found in %s\n", name, meta.Namespace)
		os.Exit(1)
	}
	out, _ := json.MarshalIndent(found, "", "  ")
	fmt.Println(string(out))
}

func summarize(meta *win32meta.NamespaceMeta) {
	fmt.Printf("namespace  %s (winmd %s, schema %d)\n", meta.Namespace, meta.WinmdVersion, meta.SchemaVersion)
	fmt.Printf("functions  %d\n", len(meta.Functions))
	fmt.Printf("structs    %d\n", len(meta.Structs))
	fmt.Printf("enums      %d\n", len(meta.Enums))
	fmt.Printf("interfaces %d\n", len(meta.Interfaces))
	fmt.Printf("delegates  %d\n", len(meta.Delegates))
	fmt.Printf("typedefs   %d\n", len(meta.Typedefs))
	fmt.Printf("constants  %d\n", len(meta.Constants))

	printSorted := func(label string, names []string) {
		sort.Strings(names)
		limit := 20
		if len(names) < limit {
			limit = len(names)
		}
		if limit > 0 {
			fmt.Printf("\n%s (first %d):\n", label, limit)
			for _, n := range names[:limit] {
				fmt.Println(" ", n)
			}
		}
	}
	var functionNames []string
	for i := range meta.Functions {
		functionNames = append(functionNames, meta.Functions[i].Name)
	}
	printSorted("functions", functionNames)
	var structNames []string
	for n := range meta.Structs {
		structNames = append(structNames, n)
	}
	printSorted("structs", structNames)
}
