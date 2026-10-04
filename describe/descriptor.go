package describe

import (
	"encoding/json"
	"fmt"

	"github.com/arisros/fate/action"
)

// LoadDescriptor unmarshals a MachineDescriptor from JSON.
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

// StateNodeDescriptor is the descriptor for a single state node.
type StateNodeDescriptor struct {
	Type          string                            `json:"type"`
	Initial       string                            `json:"initial,omitempty"`
	Default       string                            `json:"default,omitempty"`
	History       string                            `json:"history,omitempty"`
	Entry         []string                          `json:"entry,omitempty"`
	Exit          []string                          `json:"exit,omitempty"`
	On            map[string][]TransitionDescriptor `json:"on,omitempty"`
	OnDone        []TransitionDescriptor            `json:"on_done,omitempty"`
	States        map[string]StateNodeDescriptor    `json:"states,omitempty"`
	UIStateSchema json.RawMessage                   `json:"ui_state_schema,omitempty"`
	Meta          json.RawMessage                   `json:"meta,omitempty"`
}

// TransitionDescriptor is the descriptor for a single transition entry.
type TransitionDescriptor struct {
	Target   string           `json:"target,omitempty"`
	Internal bool             `json:"internal,omitempty"`
	Guard    string           `json:"guard,omitempty"`
	Actions  []string         `json:"actions,omitempty"`
	CondMeta *action.CondMeta `json:"cond_meta,omitempty"`
	Meta     json.RawMessage  `json:"meta,omitempty"`
}
