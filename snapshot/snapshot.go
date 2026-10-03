package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arisros/fate/describe"
	"github.com/arisros/fate/engine"
)

// Emit writes m.Describe() as indented JSON to <dir>/<name>.json, creating dir if needed.
func Emit[Ctx any, Evt any](dir, name string, m *engine.Machine[Ctx, Evt]) error {
	return EmitDescriptor(dir, name, m.Describe())
}

// EmitDescriptor writes a pre-built descriptor.
func EmitDescriptor(dir, name string, d describe.MachineDescriptor) error {
	if name == "" {
		return fmt.Errorf("snapshot: empty machine name")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("snapshot: mkdir %s: %w", dir, err)
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("snapshot: marshal %q: %w", name, err)
	}
	b = append(b, '\n')
	path := filepath.Join(dir, name+".json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("snapshot: write %s: %w", path, err)
	}
	return nil
}
