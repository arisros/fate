package fate

// IsKnownState reports whether `name` is a valid state name anywhere in the machine.
func (m *Machine[Ctx, Evt]) IsKnownState(name string) bool {
	if name == "" {
		return false
	}
	return m.findState(name) != nil
}

// IsTerminal reports whether `name` is a state with Type == NodeFinal.
func (m *Machine[Ctx, Evt]) IsTerminal(name string) bool {
	n := m.findState(name)
	return n != nil && n.typ == NodeFinal
}

// IsLegalTransition reports whether `eventName` declared on state `from` (or any of its ancestors, mirroring transition bubbling at runtime) has at least one candidate transition.
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

// States returns the names of every state in the machine (top-level + nested) in deterministic order: top-down, alphabetical within siblings.
func (m *Machine[Ctx, Evt]) States() []string {
	var out []string
	walkStates(m.root, &out)
	return out
}

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
	if n.name != "" {
		*out = append(*out, n.name)
	}
	keys := make([]string, 0, len(n.children))
	for k := range n.children {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, k := range keys {
		walkStates(n.children[k], out)
	}
}
