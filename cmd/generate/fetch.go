package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deploymenttheory/go-winmd/nuget"
)

const (
	nugetPackage        = "microsoft.windows.sdk.win32metadata"
	nugetPackageDisplay = "Microsoft.Windows.SDK.Win32Metadata"
	winmdFileName       = "Windows.Win32.winmd"
)

// runFetchMetadata downloads the Microsoft.Windows.SDK.Win32Metadata NuGet
// package and extracts Windows.Win32.winmd. With no --version it resolves
// the latest published version; when the committed winmd already matches,
// it exits without changes (so CI can detect updates via git diff).
func runFetchMetadata(args []string) error {
	flags := flag.NewFlagSet("fetch-metadata", flag.ExitOnError)
	version := flags.String("version", "", "package version (empty = latest published)")
	outDir := flags.String("out", filepath.Join("metadata", "winmd"), "output directory")
	force := flags.Bool("force", false, "re-download even when the version matches")
	if err := flags.Parse(args); err != nil {
		return err
	}

	client := nuget.NewClient()
	target := *version
	if target == "" {
		latest, err := nuget.LatestVersion(client, nugetPackage)
		if err != nil {
			return err
		}
		target = latest
	}

	provenancePath := filepath.Join(*outDir, "PROVENANCE.json")
	current := currentProvenance(provenancePath)
	winmdPath := filepath.Join(*outDir, winmdFileName)
	if !*force && current != nil && current.Version == target {
		if _, err := os.Stat(winmdPath); err == nil {
			fmt.Printf("up-to-date %s\n", target)
			return nil
		}
	}

	fmt.Printf("downloading %s\n", nuget.SourceURL(nugetPackage, target))
	content, record, err := nuget.Fetch(client, nugetPackage, nugetPackageDisplay, target, winmdFileName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(winmdPath, content, 0o644); err != nil {
		return err
	}
	if err := nuget.WriteProvenance(provenancePath, []nuget.Provenance{record}); err != nil {
		return err
	}
	previous := "(none)"
	if current != nil {
		previous = current.Version
	}
	fmt.Printf("updated %s -> %s (%d bytes)\n", previous, target, len(content))
	return nil
}

// currentProvenance reads the committed record for the win32 winmd, or nil.
func currentProvenance(path string) *nuget.Provenance {
	records, err := nuget.ReadProvenance(path)
	if err != nil || len(records) == 0 {
		return nil
	}
	for i := range records {
		if records[i].File == winmdFileName {
			return &records[i]
		}
	}
	return &records[0]
}
