package idiowin

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// referencesAlias reports whether code uses the import alias as a package
// qualifier (`alias.`), requiring a word boundary so that e.g. the alias
// "graphicsimaging" is not falsely matched inside "systemwinrtgraphicsimaging.".
func referencesAlias(code, alias string) bool {
	needle := alias + "."
	for from := 0; ; {
		index := strings.Index(code[from:], needle)
		if index < 0 {
			return false
		}
		position := from + index
		if position == 0 || !isIdentByte(code[position-1]) {
			return true
		}
		from = position + 1
	}
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// stripComments removes //-comment text so import-usage scans see only code.
func stripComments(body string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if index := strings.Index(line, "//"); index >= 0 {
			lines[i] = line[:index]
		}
	}
	return strings.Join(lines, "\n")
}

// writeRawFile writes pre-formatted content, creating parent directories.
func writeRawFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// pruneGeneratedTree deletes DO-NOT-EDIT-headed .go files under root that are
// not in written, then removes emptied directories. When fullSweep is false,
// only files in directories this run wrote into are considered.
func pruneGeneratedTree(root string, written map[string]bool, fullSweep bool) error {
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	writtenDirs := map[string]bool{}
	for path := range written {
		writtenDirs[filepath.Dir(path)] = true
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
		if !fullSweep && !writtenDirs[filepath.Dir(path)] {
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
