package agently

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/viant/agently-core/adapter/http/ui"
)

// loadWindowMigrations reads the operator-owned migration manifest at startup.
// It carries relocations only; target definitions and data stay in their cohort.
func loadWindowMigrations(root string) (ui.HandlerOption, error) {
	raw, err := os.ReadFile(filepath.Join(root, "resource-migrations.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read resource migrations: %w", err)
	}
	var manifest struct {
		Version int                  `json:"version"`
		Windows []ui.WindowMigration `json:"windows"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode resource migrations: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("resource migrations must contain one document")
	}
	if manifest.Version != 1 {
		return nil, fmt.Errorf("unsupported resource migration version %d", manifest.Version)
	}
	return ui.WithWindowMigrations(manifest.Windows)
}
