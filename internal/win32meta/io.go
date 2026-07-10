package win32meta

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrSchemaMismatch is returned by Read when a .w32meta.json file was written
// by an incompatible IR schema version; re-run ingest to refresh it.
var ErrSchemaMismatch = errors.New("w32meta schema version mismatch")

// FileName returns the canonical metadata file name for a namespace,
// e.g. "System.Threading.w32meta.json".
func FileName(namespace string) string {
	return namespace + ".w32meta.json"
}

// Write serializes one namespace to dir/<Namespace>.w32meta.json.
func Write(dir string, meta *NamespaceMeta) error {
	meta.SchemaVersion = CurrentSchemaVersion
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("w32meta: marshaling %s: %w", meta.Namespace, err)
	}
	data = append(data, '\n')
	path := filepath.Join(dir, FileName(meta.Namespace))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("w32meta: %w", err)
	}
	return nil
}

// Read deserializes one namespace metadata file.
func Read(path string) (*NamespaceMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("w32meta: %w", err)
	}
	var meta NamespaceMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("w32meta: parsing %s: %w", path, err)
	}
	if meta.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf("%w: %s has version %d, want %d (re-run ingest)",
			ErrSchemaMismatch, path, meta.SchemaVersion, CurrentSchemaVersion)
	}
	return &meta, nil
}

// ReadAll loads every .w32meta.json in dir, sorted by file name.
func ReadAll(dir string) ([]*NamespaceMeta, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.w32meta.json"))
	if err != nil {
		return nil, fmt.Errorf("w32meta: %w", err)
	}
	namespaces := make([]*NamespaceMeta, 0, len(paths))
	for _, path := range paths {
		meta, err := Read(path)
		if err != nil {
			return nil, err
		}
		namespaces = append(namespaces, meta)
	}
	return namespaces, nil
}
