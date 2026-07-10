// Command generate drives the go-bindings-win32 pipeline:
//
//	generate ingest    project metadata/winmd/Windows.Win32.winmd → metadata/win32/*.w32meta.json
//	generate list      list the namespaces in the committed winmd
//
// Further subcommands (bindings, idiomatic, validate, diff) arrive with later
// milestones.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	idiowin "github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/idiomatic"
	rawwin "github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw"
	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/pipeline"
	"github.com/deploymenttheory/go-bindings-win32/internal/diagnostics"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta/ingest"
	"github.com/deploymenttheory/go-bindings-win32/internal/winmd"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "fetch-metadata":
		err = runFetchMetadata(os.Args[2:])
	case "ingest":
		err = runIngest(os.Args[2:])
	case "bindings":
		err = runBindings(os.Args[2:])
	case "idiomatic":
		err = runIdiomatic(os.Args[2:])
	case "abitest":
		err = runABITest(os.Args[2:])
	case "validate":
		err = runValidate(os.Args[2:])
	case "diff":
		err = runDiff(os.Args[2:])
	case "list":
		err = runList(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "generate:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: generate <command> [flags]

commands:
  fetch-metadata  download the latest winmd from NuGet into metadata/winmd
  ingest          project the winmd into per-namespace .w32meta.json files
  bindings        emit raw Go bindings from the .w32meta.json metadata
  idiomatic       emit the idiomatic wrapper tier over the raw bindings
  abitest         generate the ABI layout acceptance test
  validate        structural integrity checks over the metadata
  diff            semantic API diff between two metadata trees
  list            list the namespaces in the winmd`)
}

// provenance mirrors metadata/winmd/PROVENANCE.json.
type provenance struct {
	Version string `json:"version"`
}

func winmdVersion(winmdPath string) string {
	data, err := os.ReadFile(filepath.Join(filepath.Dir(winmdPath), "PROVENANCE.json"))
	if err != nil {
		return ""
	}
	var p provenance
	if json.Unmarshal(data, &p) != nil {
		return ""
	}
	return p.Version
}

func runIngest(args []string) error {
	flags := flag.NewFlagSet("ingest", flag.ExitOnError)
	winmdPath := flags.String("winmd", filepath.Join("metadata", "winmd", "Windows.Win32.winmd"), "path to Windows.Win32.winmd")
	outDir := flags.String("out", filepath.Join("metadata", "win32"), "output directory for .w32meta.json files")
	namespaceFilter := flags.String("namespace", "", "comma-separated namespace filter (short names, e.g. System.Threading); empty = all")
	verbose := flags.Bool("v", false, "print diagnostics")
	if err := flags.Parse(args); err != nil {
		return err
	}

	file, err := winmd.Open(*winmdPath)
	if err != nil {
		return err
	}
	ingester := ingest.New(file, winmdVersion(*winmdPath))
	namespaces, err := ingester.Ingest()
	if err != nil {
		return err
	}

	filter := map[string]bool{}
	for _, name := range strings.Split(*namespaceFilter, ",") {
		if name = strings.TrimSpace(name); name != "" {
			filter[name] = true
		}
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	written := 0
	for _, meta := range namespaces {
		if len(filter) > 0 && !filter[meta.Namespace] {
			continue
		}
		if err := win32meta.Write(*outDir, meta); err != nil {
			return err
		}
		written++
	}
	if *verbose {
		for _, diagnostic := range ingester.Diagnostics {
			fmt.Fprintln(os.Stderr, "diagnostic:", diagnostic)
		}
	}
	fmt.Printf("ingested %d namespaces → %s (%d diagnostics)\n", written, *outDir, len(ingester.Diagnostics))
	return nil
}

func runBindings(args []string) error {
	flags := flag.NewFlagSet("bindings", flag.ExitOnError)
	metadataDir := flags.String("metadata", filepath.Join("metadata", "win32"), "directory of .w32meta.json files")
	outDir := flags.String("out", filepath.Join("bindings", "win32"), "output root for generated packages")
	namespaceFilter := flags.String("namespace", "", "comma-separated namespace filter; empty = all loaded")
	verbose := flags.Bool("v", false, "print diagnostics")
	writeBaseline := flags.String("diagnostics", "", "write the diagnostics baseline to this path")
	checkBaseline := flags.String("diagnostics-baseline", "", "fail if any diagnostic is not in this committed baseline")
	if err := flags.Parse(args); err != nil {
		return err
	}

	registry, err := pipeline.LoadAll(*metadataDir)
	if err != nil {
		return err
	}
	filter := map[string]bool{}
	for _, name := range strings.Split(*namespaceFilter, ",") {
		if name = strings.TrimSpace(name); name != "" {
			filter[name] = true
		}
	}
	generator := rawwin.New(registry, modulePath, *outDir)
	written, err := generator.EmitAll(filter)
	if err != nil {
		return err
	}
	if *verbose {
		for _, diagnostic := range generator.Diagnostics {
			fmt.Fprintln(os.Stderr, "diagnostic:", diagnostic)
		}
	}
	fmt.Printf("emitted %d packages → %s (%d diagnostics)\n", written, *outDir, len(generator.Diagnostics))

	if *writeBaseline != "" {
		if err := diagnostics.WriteBaseline(*writeBaseline, generator.Diagnostics); err != nil {
			return err
		}
		fmt.Printf("wrote diagnostics baseline → %s\n", *writeBaseline)
	}
	if *checkBaseline != "" {
		newEntries, err := diagnostics.CheckBaseline(*checkBaseline, generator.Diagnostics)
		if err != nil {
			return err
		}
		if len(newEntries) > 0 {
			for _, entry := range newEntries {
				fmt.Fprintln(os.Stderr, "new diagnostic:", entry)
			}
			return fmt.Errorf("%d diagnostics beyond baseline %s (fix them, or rewrite the baseline with --diagnostics after review)",
				len(newEntries), *checkBaseline)
		}
		fmt.Println("diagnostics within baseline")
	}
	return nil
}

// modulePath is this module's import path root.
const modulePath = "github.com/deploymenttheory/go-bindings-win32"

// runIdiomatic emits the idiomatic tier. It first runs the raw emitter (into
// a scratch dir it discards) to learn which functions were emitted and to
// share the mapper's degradation decisions, then emits the wrappers.
func runIdiomatic(args []string) error {
	flags := flag.NewFlagSet("idiomatic", flag.ExitOnError)
	metadataDir := flags.String("metadata", filepath.Join("metadata", "win32"), "directory of .w32meta.json files")
	rawDir := flags.String("raw", filepath.Join("bindings", "win32"), "raw bindings root (probed for emitted functions)")
	outDir := flags.String("out", filepath.Join("opinionated", "idiomatic", "win32"), "output root")
	verbose := flags.Bool("v", false, "print diagnostics")
	if err := flags.Parse(args); err != nil {
		return err
	}
	registry, err := pipeline.LoadAll(*metadataDir)
	if err != nil {
		return err
	}
	// Probe the raw tier: emit it to discover the emitted-function set and
	// populate the shared mapper, writing to the real raw dir (idempotent).
	rawGen := rawwin.New(registry, modulePath, *rawDir)
	if _, err := rawGen.EmitAll(nil); err != nil {
		return err
	}
	idioGen := idiowin.New(registry, rawGen.Mapper(), rawGen.EmittedFunctions(), modulePath, *outDir)
	written, err := idioGen.EmitAll()
	if err != nil {
		return err
	}
	if *verbose {
		for _, diagnostic := range idioGen.Diagnostics {
			fmt.Fprintln(os.Stderr, "diagnostic:", diagnostic)
		}
	}
	fmt.Printf("emitted %d idiomatic packages → %s (%d diagnostics)\n", written, *outDir, len(idioGen.Diagnostics))
	return nil
}

// runABITest regenerates all bindings (collecting expected struct layouts)
// and writes the sampled ABI acceptance test.
func runABITest(args []string) error {
	flags := flag.NewFlagSet("abitest", flag.ExitOnError)
	metadataDir := flags.String("metadata", filepath.Join("metadata", "win32"), "directory of .w32meta.json files")
	outDir := flags.String("out", filepath.Join("bindings", "win32"), "bindings output root")
	testPath := flags.String("test-out", filepath.Join("acceptance", "abi_generated_test.go"), "generated test path")
	sample := flags.Int("sample", 400, "approximate number of sampled structs (Foundation always included)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	registry, err := pipeline.LoadAll(*metadataDir)
	if err != nil {
		return err
	}
	generator := rawwin.New(registry, modulePath, *outDir)
	if _, err := generator.EmitAll(nil); err != nil {
		return err
	}
	source := rawwin.BuildABITest(generator.ABIRecords(), modulePath, *sample)
	if err := os.WriteFile(*testPath, []byte(source), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d structs recorded)\n", *testPath, len(generator.ABIRecords()))
	return nil
}

func runList(args []string) error {
	flags := flag.NewFlagSet("list", flag.ExitOnError)
	winmdPath := flags.String("winmd", filepath.Join("metadata", "winmd", "Windows.Win32.winmd"), "path to Windows.Win32.winmd")
	if err := flags.Parse(args); err != nil {
		return err
	}
	file, err := winmd.Open(*winmdPath)
	if err != nil {
		return err
	}
	ingester := ingest.New(file, "")
	namespaces, err := ingester.Ingest()
	if err != nil {
		return err
	}
	for _, meta := range namespaces {
		fmt.Printf("%-60s %5d funcs %5d structs %5d enums %5d ifaces %6d consts\n",
			meta.Namespace, len(meta.Functions), len(meta.Structs), len(meta.Enums),
			len(meta.Interfaces), len(meta.Constants))
	}
	return nil
}
