package fate

// MachineDescriptor is a JSON-serializable view of a validated Machine.
// It strips the generic Ctx/Evt parameters so that loaders (e.g. the TUI
// studio in P7, doc generators, divergence reporters) can introspect any
// machine's topology without needing to know the concrete types.
//
// Guards and actions appear by their declared name — the descriptor never
// executes them. Code that wants to step an actor must hold the original
// `*Machine[Ctx, Evt]` instance.

import (
	"encoding/json"
	"fmt"
	"sort"
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
}

// TransitionDescriptor is the descriptor for a single transition entry.
// Guards / actions appear as names only.
type TransitionDescriptor struct {
	Target   string   `json:"target,omitempty"`
	Internal bool     `json:"internal,omitempty"`
	Guard    string   `json:"guard,omitempty"`
	Actions  []string `json:"actions,omitempty"`
}

// Describe returns a MachineDescriptor for the machine. The context is
// JSON-marshaled if possible; on marshal failure (e.g. a Ctx containing a
// channel) the Context field is left nil and the rest of the descriptor
// still renders correctly.
//
// Action and Guard names come from each value's ImplName() method when
// implemented, falling back to "" otherwise. Anonymous closures therefore
// show as empty strings — callers that care should name their actions
// (see actions.go for helpers like Named, Assign).
func (m *Machine[Ctx, Evt]) Describe() MachineDescriptor {
	d := MachineDescriptor{
		ID:      m.id,
		Initial: m.root.initial,
		States:  map[string]StateNodeDescriptor{},
	}
	if ctxBytes, err := json.Marshal(m.context); err == nil {
		// "null" is the marshaled form of an unset interface or zero value
		// with no fields; omit it for cleaner output.
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
		// Sort event keys for deterministic descriptor output.
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
		if t.Guard != nil {
			td.Guard = guardName(t.Guard)
		}
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

// actionName extracts a human-readable name for an action. Falls back to
// "" when the value doesn't expose one.
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

// guardName extracts a human-readable name for a guard. Guard is a func
// value with no interface method; the studio descriptor surfaces empty
// strings for anonymous guards. Callers needing named guards should wrap
// the closure in a struct that implements ImplName().
func guardName[Ctx any, Evt any](g Guard[Ctx, Evt]) string {
	if g == nil {
		return ""
	}
	type named interface{ ImplName() string }
	if n, ok := any(g).(named); ok {
		return n.ImplName()
	}
	return ""
}
