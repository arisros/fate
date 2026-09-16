package fate

import (
	"encoding/json"
	"fmt"
	"sort"
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
	Type    string                            `json:"type"`
	Initial string                            `json:"initial,omitempty"`
	Default string                            `json:"default,omitempty"`
	History string                            `json:"history,omitempty"`
	Entry   []string                          `json:"entry,omitempty"`
	Exit    []string                          `json:"exit,omitempty"`
	On      map[string][]TransitionDescriptor `json:"on,omitempty"`
	OnDone  []TransitionDescriptor            `json:"on_done,omitempty"`
	States  map[string]StateNodeDescriptor    `json:"states,omitempty"`
}

// TransitionDescriptor is the descriptor for a single transition entry.
type TransitionDescriptor struct {
	Target   string   `json:"target,omitempty"`
	Internal bool     `json:"internal,omitempty"`
	Guard    string   `json:"guard,omitempty"`
	Actions  []string `json:"actions,omitempty"`
}

// Describe returns a MachineDescriptor for the machine.
func (m *Machine[Ctx, Evt]) Describe() MachineDescriptor {
	d := MachineDescriptor{
		ID:      m.id,
		Initial: m.root.initial,
		States:  map[string]StateNodeDescriptor{},
	}
	if ctxBytes, err := json.Marshal(m.context); err == nil {
		if string(ctxBytes) != "null" {
			d.Context = ctxBytes
		}
	}
	for name, child := range m.root.children {
		d.States[name] = describeNode(child)
	}
	return d
}

func describeNode[Ctx any, Evt any](n *stateNode[Ctx, Evt]) StateNodeDescriptor {
	sd := StateNodeDescriptor{
		Type:    n.typ.String(),
		Initial: n.initial,
	}
	if n.typ == NodeHistory {
		sd.Default = n.defaultTgt
		switch n.history {
		case HistoryDeep:
			sd.History = "deep"
		default:
			sd.History = "shallow"
		}
	}
	if names := describeActions(n.entryActions); len(names) > 0 {
		sd.Entry = names
	}
	if names := describeActions(n.exitActions); len(names) > 0 {
		sd.Exit = names
	}
	if len(n.on) > 0 {
		sd.On = map[string][]TransitionDescriptor{}
		eventKeys := make([]string, 0, len(n.on))
		for k := range n.on {
			eventKeys = append(eventKeys, k)
		}
		sort.Strings(eventKeys)
		for _, ev := range eventKeys {
			sd.On[ev] = describeTransitions(n.on[ev])
		}
	}
	if len(n.onDone) > 0 {
		sd.OnDone = describeTransitions(n.onDone)
	}
	if len(n.children) > 0 {
		sd.States = map[string]StateNodeDescriptor{}
		for name, child := range n.children {
			sd.States[name] = describeNode(child)
		}
	}
	return sd
}

func describeTransitions[Ctx any, Evt any](ts []TransitionConfig[Ctx, Evt]) []TransitionDescriptor {
	out := make([]TransitionDescriptor, 0, len(ts))
	for _, t := range ts {
		td := TransitionDescriptor{
			Target:   t.Target,
			Internal: t.Internal,
		}
		td.Guard = t.GuardName
		if names := describeActions(t.Actions); len(names) > 0 {
			td.Actions = names
		}
		out = append(out, td)
	}
	return out
}

func describeActions[Ctx any, Evt any](actions []Action[Ctx, Evt]) []string {
	if len(actions) == 0 {
		return nil
	}
	names := make([]string, 0, len(actions))
	for _, a := range actions {
		names = append(names, actionName(a))
	}
	return names
}

func actionName[Ctx any, Evt any](a Action[Ctx, Evt]) string {
	if a == nil {
		return ""
	}
	type named interface{ ImplName() string }
	if n, ok := any(a).(named); ok {
		return n.ImplName()
	}
	return ""
}
