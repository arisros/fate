package engine

// Stateless validator helpers on *Machine — exposed for callers that need
// to ask FSM questions WITHOUT spinning up an Actor instance.
//
// Primary use case is a status machine bolted
// to a document field as a write-time validator. It needs:
//
//   - IsKnownState(name)         — plain set-membership check
//                                  over every state, nested ones included.
//   - IsLegalTransition(from,    — strict transition reachability check,
//                       eventName) optional tighter validator for callers
//                                  that opt into it.
//   - IsTerminal(name)           — termination predicate (true for final
//                                  states only).
//   - States()                   — full state-name list for schema-vs-machine
//                                  enum sync checks, where a mismatch
//                                  should fail at startup.
//
// All methods walk the immutable *Machine and are safe to call concurrently.

// IsKnownState reports whether `name` is a valid state name anywhere in the
// machine. The check is recursive — it matches both top-level states and
// nested children. This is plain set membership, the
// loosest of the validators here.
func (m *Machine[Ctx, Evt]) IsKnownState(name string) bool {
	if name == "" {
		return false
	}
	return m.findState(name) != nil
}

// IsTerminal reports whether `name` is a state with Type == NodeFinal.
// It needs no Actor instance.
func (m *Machine[Ctx, Evt]) IsTerminal(name string) bool {
	n := m.findState(name)
	return n != nil && n.typ == NodeFinal
}

// IsLegalTransition reports whether `eventName` declared on state `from`
// (or any of its ancestors, mirroring transition bubbling at runtime) has
// at least one candidate transition. It does NOT evaluate guards — guards
// require an event payload and context, neither of which are available here.
//
// Use this when you want stricter-than-set-membership validation. The
// set-membership check (IsKnownState) stays the default for
// backward compatibility; opt into
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
// Used by schema-vs-machine enum sync checks (a status enum that has to match
// machine states exactly).
func (m *Machine[Ctx, Evt]) States() []string {
	var out []string
	walkStates(m.root, &out)
	return out
}

// findState returns the first state node whose local `name` matches.
// Searches breadth-first to favor top-level matches when names collide
// (they shouldn't in well-formed machines, but the search is defined).
//
// Children are visited in alphabetical order. Without that, two states sharing
// a name at the same depth would resolve to whichever the map happened to yield
// first, so IsKnownState, IsTerminal and IsLegalTransition could disagree with
// themselves between runs of the same program.
func (m *Machine[Ctx, Evt]) findState(name string) *stateNode[Ctx, Evt] {
	queue := []*stateNode[Ctx, Evt]{m.root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n.name == name {
			return n
		}
		childNames := make([]string, 0, len(n.children))
		for childName := range n.children {
			childNames = append(childNames, childName)
		}
		sortStrings(childNames)
		for _, childName := range childNames {
			queue = append(queue, n.children[childName])
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
