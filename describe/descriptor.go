package describe

import (
	"encoding/json"
	"fmt"

	"github.com/arisros/fate/action"
)

// LoadDescriptor unmarshals a MachineDescriptor from JSON. Used by the TUI
// studio's static-view mode (P7) to load a machine without compiling Go
// code: the workflow team can `go run ./cmd/dump-descriptors` against a
// service, save the JSON output, and inspect it elsewhere.
//
// Validation is shape-only — non-empty ID, at least one state, every
// state's type is a recognized string. Round-trip with Describe() is the
// authoritative contract; the function is intentionally permissive about
// extra fields (forward compatibility for future descriptor versions).
func LoadDescriptor(data []byte) (MachineDescriptor, error) {
	var d MachineDescriptor
	if err := json.Unmarshal(data, &d); err != nil {
		return MachineDescriptor{}, fmt.Errorf("statechart: descriptor unmarshal: %w", err)
	}
	if d.ID == "" {
		return MachineDescriptor{}, fmt.Errorf("statechart: descriptor missing 'id'")
	}
	if len(d.States) == 0 {
		return MachineDescriptor{}, fmt.Errorf("statechart: descriptor has no states")
	}
	if err := validateDescriptorStates(d.States); err != nil {
		return MachineDescriptor{}, err
	}
	return d, nil
}

func validateDescriptorStates(states map[string]StateNodeDescriptor) error {
	for name, s := range states {
		switch s.Type {
		case "atomic", "compound", "parallel", "final", "history":
			// ok
		default:
			return fmt.Errorf("statechart: state %q has unknown type %q", name, s.Type)
		}
		if len(s.States) > 0 {
			if err := validateDescriptorStates(s.States); err != nil {
				return err
			}
		}
	}
	return nil
}

// MachineDescriptor is the root of the descriptor tree.
type MachineDescriptor struct {
	ID      string                         `json:"id"`
	Initial string                         `json:"initial"`
	Context json.RawMessage                `json:"context,omitempty"`
	States  map[string]StateNodeDescriptor `json:"states"`
}

// StateNodeDescriptor is the descriptor for a single state node. Mirrors
// StateNodeConfig but with strings where the original held function values
// or generic actions.
type StateNodeDescriptor struct {
	Type    string                            `json:"type"` // "atomic" | "compound" | "parallel" | "final" | "history"
	Initial string                            `json:"initial,omitempty"`
	Default string                            `json:"default,omitempty"` // history default target
	History string                            `json:"history,omitempty"` // "shallow" | "deep" (only for history nodes)
	Entry   []string                          `json:"entry,omitempty"`   // action names
	Exit    []string                          `json:"exit,omitempty"`    // action names
	On      map[string][]TransitionDescriptor `json:"on,omitempty"`
	OnDone  []TransitionDescriptor            `json:"on_done,omitempty"`
	States  map[string]StateNodeDescriptor    `json:"states,omitempty"`
	// UIStateSchema is the JSON Schema of the state's UIState view model.
	UIStateSchema json.RawMessage `json:"ui_state_schema,omitempty"`
	// Meta is the state's StateNodeConfig.Meta as a JSON object.
	Meta json.RawMessage `json:"meta,omitempty"`
}

// TransitionDescriptor is the descriptor for a single transition entry.
// Guards / actions appear as names only.
type TransitionDescriptor struct {
	Target   string           `json:"target,omitempty"`
	Internal bool             `json:"internal,omitempty"`
	Guard    string           `json:"guard,omitempty"`
	Actions  []string         `json:"actions,omitempty"`
	CondMeta *action.CondMeta `json:"cond_meta,omitempty"`
	// Meta is the transition's TransitionConfig.Meta as a JSON object.
	Meta json.RawMessage `json:"meta,omitempty"`
}
