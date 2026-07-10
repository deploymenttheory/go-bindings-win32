package idiowin

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// writeRawFile writes pre-formatted content, creating parent directories.
func writeRawFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// pruneGeneratedTree deletes DO-NOT-EDIT-headed .go files under root that are
// not in written, then removes emptied directories.
func pruneGeneratedTree(root string, written map[string]bool) error {
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		if !strings.HasSuffix(path, ".go") || written[path] {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if !strings.HasPrefix(string(content), generatedHeader) {
			return nil
		}
		return os.Remove(path)
	})
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		if entries, readErr := os.ReadDir(dir); readErr == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
	return nil
}
