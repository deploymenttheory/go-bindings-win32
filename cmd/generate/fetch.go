package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const nugetPackage = "microsoft.windows.sdk.win32metadata"

// fullProvenance is the complete metadata/winmd/PROVENANCE.json shape.
type fullProvenance struct {
	Package string `json:"package"`
	Version string `json:"version"`
	Source  string `json:"source"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
	Fetched string `json:"fetched"`
}

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

	client := &http.Client{Timeout: 5 * time.Minute}
	target := *version
	if target == "" {
		latest, err := latestVersion(client)
		if err != nil {
			return err
		}
		target = latest
	}

	current := readProvenance(filepath.Join(*outDir, "PROVENANCE.json"))
	winmdPath := filepath.Join(*outDir, "Windows.Win32.winmd")
	if !*force && current != nil && current.Version == target {
		if _, err := os.Stat(winmdPath); err == nil {
			fmt.Printf("up-to-date %s\n", target)
			return nil
		}
	}

	sourceURL := fmt.Sprintf("https://api.nuget.org/v3-flatcontainer/%s/%s/%s.%s.nupkg",
		nugetPackage, target, nugetPackage, target)
	fmt.Printf("downloading %s\n", sourceURL)
	nupkg, err := httpGet(client, sourceURL)
	if err != nil {
		return err
	}

	winmd, err := extractWinmd(nupkg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(winmdPath, winmd, 0o644); err != nil {
		return err
	}
	updated := fullProvenance{
		Package: "Microsoft.Windows.SDK.Win32Metadata",
		Version: target,
		Source:  sourceURL,
		File:    "Windows.Win32.winmd",
		SHA256:  fmt.Sprintf("%x", sha256.Sum256(winmd)),
		Fetched: time.Now().UTC().Format("2006-01-02"),
	}
	data, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "PROVENANCE.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	previous := "(none)"
	if current != nil {
		previous = current.Version
	}
	fmt.Printf("updated %s -> %s (%d bytes)\n", previous, target, len(winmd))
	return nil
}

// latestVersion resolves the newest published package version.
func latestVersion(client *http.Client) (string, error) {
	indexURL := fmt.Sprintf("https://api.nuget.org/v3-flatcontainer/%s/index.json", nugetPackage)
	data, err := httpGet(client, indexURL)
	if err != nil {
		return "", err
	}
	var index struct {
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return "", fmt.Errorf("parsing NuGet version index: %w", err)
	}
	if len(index.Versions) == 0 {
		return "", fmt.Errorf("NuGet index lists no versions for %s", nugetPackage)
	}
	// The flat-container index is ordered oldest → newest.
	return index.Versions[len(index.Versions)-1], nil
}

func httpGet(client *http.Client, url string) ([]byte, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	return data, nil
}

// extractWinmd pulls Windows.Win32.winmd out of the nupkg (a zip).
func extractWinmd(nupkg []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(nupkg), int64(len(nupkg)))
	if err != nil {
		return nil, fmt.Errorf("opening nupkg: %w", err)
	}
	for _, file := range archive.File {
		if file.Name != "Windows.Win32.winmd" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	return nil, fmt.Errorf("nupkg contains no Windows.Win32.winmd")
}

func readProvenance(path string) *fullProvenance {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var p fullProvenance
	if json.Unmarshal(data, &p) != nil {
		return nil
	}
	return &p
}
