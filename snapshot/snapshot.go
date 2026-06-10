// Package snapshot writes fate machine descriptors to .fate/*.json files for
// consumption by fate-studio's snapshot viewer.
//
// The emitted file is a plain MachineDescriptor JSON — a type-erased, engine-
// runtime-free view of the machine's structure. fate-studio loads it with
// fate.LoadDescriptor and renders the chart WITHOUT importing or running the
// implementation, so a core (LTW, fate-example, …) whose machines live in
// internal packages can still be visualized across module boundaries.
//
// Wire it into a build via go:generate, e.g. from a small generator main:
//
//	//go:generate go run ./internal/gen
//	snapshot.Emit("./.fate", "survey", survey.NewMachine())
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/arisros/fate"
)

// Emit writes m.Describe() as indented JSON to <dir>/<name>.json, creating dir
// if needed. name becomes the studio machine id, so keep it stable and
// filesystem-safe (lowercase, hyphens).
func Emit[Ctx any, Evt any](dir, name string, m *fate.Machine[Ctx, Evt]) error {
	return EmitDescriptor(dir, name, m.Describe())
}

// EmitDescriptor writes a pre-built descriptor. Use when you already hold a
// descriptor (e.g. one loaded from elsewhere) rather than a live *Machine.
func EmitDescriptor(dir, name string, d fate.MachineDescriptor) error {
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
