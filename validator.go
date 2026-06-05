package fate

// Stateless validator helpers on *Machine — exposed for callers that need
// to ask FSM questions WITHOUT spinning up an Actor instance.
//
// Primary use case is the LPW Application Status FSM (P12) which is bolted
// to a document field as a write-time validator. It needs:
//
//   - IsKnownState(name)         — preserves current fp.StateMachine
//                                  AsStateValidator semantics (set membership).
//   - IsLegalTransition(from,    — strict transition reachability check,
//                       eventName) optional tighter validator for callers
//                                  that opt into it.
//   - IsTerminal(name)           — termination predicate (replaces the legacy
//                                  IsTerminalStatus consumer in termination.go).
//   - States()                   — full state-name list for schema-vs-FSM
//                                  enum sync checks (panic-on-mismatch case
//                                  documented in fsm-lpw-application-status.md).
//
// All methods walk the immutable *Machine and are safe to call concurrently.

// IsKnownState reports whether `name` is a valid state name anywhere in the
// machine. The check is recursive — it matches both top-level states and
// nested children. This mirrors the legacy fp.StateMachine.AsStateValidator
// behavior used by LPW.
func (m *Machine[Ctx, Evt]) IsKnownState(name string) bool {
	if name == "" {
		return false
	}
	return m.findState(name) != nil
}

// IsTerminal reports whether `name` is a state with Type == NodeFinal.
// Replaces the legacy IsTerminalStatus consumer in termination.go.
func (m *Machine[Ctx, Evt]) IsTerminal(name string) bool {
	n := m.findState(name)
	return n != nil && n.typ == NodeFinal
}

// IsLegalTransition reports whether `eventName` declared on state `from`
// (or any of its ancestors, mirroring transition bubbling at runtime) has
// at least one candidate transition. It does NOT evaluate guards — guards
// require an event payload and context, neither of which are available here.
//
// Use this when you want stricter-than-set-membership validation. The LPW
// port keeps the legacy set-membership default (via IsKnownState) for
// backward compat per migration-playbook P12 decision; opt into
// IsLegalTransition where stricter checks are wanted.
func (m *Machine[Ctx, Evt]) IsLegalTransition(from string, eventName string) bool {
	n := m.findState(from)
	if n == nil {
		return false
	}
	for cursor := n; cursor != nil; cursor = cursor.parent {
		if len(cursor.on[eventName]) > 0 {
			return true
		}
	}
	return false
}

// States returns the names of every state in the machine (top-level +
// nested) in deterministic order: top-down, alphabetical within siblings.
// Used by schema-vs-FSM enum sync checks (LPW expects status enum to match
// machine states exactly).
func (m *Machine[Ctx, Evt]) States() []string {
	var out []string
	walkStates(m.root, &out)
	return out
}

// findState returns the first state node whose local `name` matches.
// Searches breadth-first to favor top-level matches when names collide
// (they shouldn't in well-formed machines, but the search is defined).
func (m *Machine[Ctx, Evt]) findState(name string) *stateNode[Ctx, Evt] {
	queue := []*stateNode[Ctx, Evt]{m.root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.name == name {
			return n
		}
		for _, child := range n.children {
			queue = append(queue, child)
		}
	}
	return nil
}

func walkStates[Ctx any, Evt any](n *stateNode[Ctx, Evt], out *[]string) {
	if n == nil {
		return
	}
	if n.name != "" { // skip synthetic root
		*out = append(*out, n.name)
	}
	// Deterministic order: alphabetical.
	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		walkStates(n.children[k], out)
	}
}
