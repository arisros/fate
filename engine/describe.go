package engine

import (
	"encoding/json"
	"slices"
	"sort"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/describe"
)

// Describe returns a MachineDescriptor for the machine. The context is
// JSON-marshaled if possible; on marshal failure (e.g. a Ctx containing a
// channel) the Context field is left nil and the rest of the descriptor
// still renders correctly.
//
// Action names come from each value's ImplName() method: the built-in actions
// report their kind ("assign", "raise:CANCEL", "log"), and [action.Named] attaches a
// caller-chosen label. Guard names come from [TransitionConfig.GuardName],
// since a func value carries no name a descriptor could recover. Anything
// unnamed falls back to "". A Raise names its event the way Send would, so
// with MachineConfig.EventName set the label carries the name it returns.
func (m *Machine[Ctx, Evt]) Describe() describe.MachineDescriptor {
	d := describe.MachineDescriptor{
		ID:      m.id,
		Initial: m.root.initial,
		States:  map[string]describe.StateNodeDescriptor{},
	}
	if ctxBytes, err := json.Marshal(m.context); err == nil {
		// "null" is the marshaled form of an unset interface or zero value
		// with no fields; omit it for cleaner output.
		if string(ctxBytes) != "null" {
			d.Context = ctxBytes
		}
	}
	for name, child := range m.root.children {
		d.States[name] = m.describeNode(child)
	}
	return d
}

func (m *Machine[Ctx, Evt]) describeNode(n *stateNode[Ctx, Evt]) describe.StateNodeDescriptor {
	sd := describe.StateNodeDescriptor{
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
	if names := m.describeActions(n.entryActions); len(names) > 0 {
		sd.Entry = names
	}
	if names := m.describeActions(n.exitActions); len(names) > 0 {
		sd.Exit = names
	}
	if len(n.on) > 0 {
		sd.On = map[string][]describe.TransitionDescriptor{}
		// Sort event keys for deterministic descriptor output.
		eventKeys := make([]string, 0, len(n.on))
		for k := range n.on {
			eventKeys = append(eventKeys, k)
		}
		sort.Strings(eventKeys)
		for _, ev := range eventKeys {
			sd.On[ev] = m.describeTransitions(n.on[ev])
		}
	}
	if len(n.onDone) > 0 {
		sd.OnDone = m.describeTransitions(n.onDone)
	}
	if n.uiState != nil {
		sd.UIStateSchema = n.uiState.Schema()
	}
	sd.Meta = slices.Clone(n.meta)
	if len(n.children) > 0 {
		sd.States = map[string]describe.StateNodeDescriptor{}
		for name, child := range n.children {
			sd.States[name] = m.describeNode(child)
		}
	}
	return sd
}

func (m *Machine[Ctx, Evt]) describeTransitions(ts []TransitionConfig[Ctx, Evt]) []describe.TransitionDescriptor {
	out := make([]describe.TransitionDescriptor, 0, len(ts))
	open := firstUnconditional(ts)
	for i, t := range ts {
		td := describe.TransitionDescriptor{
			Target:   t.Target,
			Internal: t.Internal,
			CondMeta: t.CondMeta.Clone(),
			Fallback: i == open && i > 0,
			Shadowed: open >= 0 && i > open,
			Meta:     slices.Clone(t.meta),
		}
		td.Guard = t.GuardName
		if names := m.describeActions(t.Actions); len(names) > 0 {
			td.Actions = names
		}
		out = append(out, td)
	}
	return out
}

func (m *Machine[Ctx, Evt]) describeActions(actions []action.Action[Ctx, Evt]) []string {
	if len(actions) == 0 {
		return nil
	}
	names := make([]string, 0, len(actions))
	for _, a := range actions {
		names = append(names, m.actionName(a))
	}
	return names
}

// actionName extracts a human-readable name for an action. Falls back to
// "" when the value doesn't expose one. A Raise is labelled with the name its
// event dispatches on, so the label matches the On key that handles it.
func (m *Machine[Ctx, Evt]) actionName(a action.Action[Ctx, Evt]) string {
	if a == nil {
		return ""
	}
	if r, ok := any(a).(interface{ RaisedEvent() Evt }); ok {
		if name, err := m.eventName(r.RaisedEvent()); err == nil {
			return "raise:" + name
		}
		return "raise"
	}
	type named interface{ ImplName() string }
	if n, ok := any(a).(named); ok {
		return n.ImplName()
	}
	return ""
}
