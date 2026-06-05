package fate

// SelectedTransition records the outcome of selectTransitions per active
// leaf: the resolved source node and the matching transition config.
type SelectedTransition[Ctx any, Evt any] struct {
	Source *stateNode[Ctx, Evt]
	Config TransitionConfig[Ctx, Evt]
}

// selectTransitions returns every transition that fires for `evt`, one per
// active region whose leaf bubbles up to a matching handler. For
// non-parallel configurations this returns at most one entry; for parallel
// configurations multiple regions can fire on the same event.
//
// A handler declared on a shared ancestor (e.g. on the parallel node itself)
// is registered exactly once even if reached from multiple leaves.
func selectTransitions[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	current StateValue,
	ctx Ctx,
	evt Evt,
	eventName string,
) []SelectedTransition[Ctx, Evt] {
	leaves := resolveLeaves[Ctx, Evt](root, current)
	seen := map[*stateNode[Ctx, Evt]]bool{}
	var out []SelectedTransition[Ctx, Evt]
	for _, leaf := range leaves {
		for cursor := leaf; cursor != nil && cursor.name != ""; cursor = cursor.parent {
			if seen[cursor] {
				break
			}
			candidates := cursor.on[eventName]
			if len(candidates) == 0 {
				candidates = cursor.on["*"]
			}
			matched := false
			for _, t := range candidates {
				if transitionPasses(t, ctx, evt, current) {
					seen[cursor] = true
					out = append(out, SelectedTransition[Ctx, Evt]{Source: cursor, Config: t})
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
	}
	return out
}

// resolveLeaf returns the first leaf for value `v`. For parallel
// configurations there are multiple active leaves; resolveLeaf returns one
// deterministic choice (alphabetically-first region) so legacy single-leaf
// call sites still produce reasonable output. Use resolveLeaves for
// parallel-aware iteration.
func resolveLeaf[Ctx any, Evt any](root *stateNode[Ctx, Evt], v StateValue) *stateNode[Ctx, Evt] {
	leaves := resolveLeaves[Ctx, Evt](root, v)
	if len(leaves) == 0 {
		return nil
	}
	return leaves[0]
}

// resolveLeaves walks a StateValue against the validated node tree and
// returns every active leaf state node, in deterministic (alphabetical)
// path order. For non-parallel configurations the slice has length 1.
func resolveLeaves[Ctx any, Evt any](root *stateNode[Ctx, Evt], v StateValue) []*stateNode[Ctx, Evt] {
	var out []*stateNode[Ctx, Evt]
	walkValue[Ctx, Evt](root, v, &out)
	return out
}

// walkValue descends into v against parent and appends every reached leaf
// node (atomic or final) to *out. Map keys are visited in alphabetical
// order so the resulting slice is deterministic.
func walkValue[Ctx any, Evt any](parent *stateNode[Ctx, Evt], v StateValue, out *[]*stateNode[Ctx, Evt]) {
	// If we've recursed all the way into a terminal-shaped node, we ARE the
	// leaf; any redundant value (e.g. AtomicValue(parent.name) from an
	// atomic parallel region) is ignored.
	if parent.typ == NodeAtomic || parent.typ == NodeFinal || parent.typ == NodeHistory {
		*out = append(*out, parent)
		return
	}
	if v.IsAtomic() {
		child, ok := parent.children[v.Leaf]
		if !ok {
			return
		}
		// child is the leaf candidate. If child is itself compound or
		// parallel (e.g., when a value-fragment is just a name and the
		// child has further structure), descend into its initial chain.
		switch child.typ {
		case NodeAtomic, NodeFinal, NodeHistory:
			*out = append(*out, child)
		case NodeCompound:
			if init, ok := child.children[child.initial]; ok {
				walkValue[Ctx, Evt](child, AtomicValue(init.name), out)
			}
		case NodeParallel:
			walkValue[Ctx, Evt](child, child.initialInner(), out)
		}
		return
	}
	// Compound or parallel value: visit children in alphabetical order.
	keys := make([]string, 0, len(v.Children))
	for k := range v.Children {
		keys = append(keys, k)
	}
	sortStrings(keys)
	for _, name := range keys {
		childValue := v.Children[name]
		child, ok := parent.children[name]
		if !ok {
			continue
		}
		walkValue[Ctx, Evt](child, childValue, out)
	}
}

// sortStrings is a sort wrapper used inline so we don't import "sort" in
// every transition pass. (Inlined since the import is already in machine.go.)
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// commitValue computes the actor's new value after a transition lands on
// `target`, preserving unaffected parallel-region siblings from `current`.
//
// Walks down from root via target.path; at each step:
//
//   - Compound parent: replace the single active child with the new branch.
//   - Parallel parent: preserve sibling regions from `current`; replace only
//     the region containing target.
//   - Atomic active leaf inside a compound parent: encoded as
//     AtomicValue(leafName) (no extra Children wrap).
func commitValue[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	current StateValue,
	target *stateNode[Ctx, Evt],
) StateValue {
	if len(target.path) == 0 {
		return current
	}
	return commitDescend[Ctx, Evt](root, current, target.path, target)
}

// commitDescend returns the value-INSIDE `parent` after applying the path.
func commitDescend[Ctx any, Evt any](
	parent *stateNode[Ctx, Evt],
	currentInside StateValue,
	pathRemaining []string,
	target *stateNode[Ctx, Evt],
) StateValue {
	if len(pathRemaining) == 0 {
		return target.initialInner()
	}
	nextName := pathRemaining[0]
	nextNode, ok := parent.children[nextName]
	if !ok {
		return currentInside
	}
	rest := pathRemaining[1:]

	var newChildEntry StateValue
	if len(rest) == 0 {
		newChildEntry = nextNode.initialInner()
	} else {
		var nestedCurrent StateValue
		if currentInside.Children != nil {
			nestedCurrent = currentInside.Children[nextName]
		}
		newChildEntry = commitDescend[Ctx, Evt](nextNode, nestedCurrent, rest, target)
	}

	switch parent.typ {
	case NodeParallel:
		regions := make(map[string]StateValue, len(parent.children))
		for childName, childNode := range parent.children {
			if childName == nextName {
				regions[childName] = newChildEntry
				continue
			}
			if currentInside.Children != nil {
				if existing, ok := currentInside.Children[childName]; ok {
					regions[childName] = existing
					continue
				}
			}
			regions[childName] = childNode.initialInner()
		}
		return StateValue{Children: regions}
	default:
		// Compound parent (including synthetic root). When the new active
		// child is atomic-shaped, parent's value-inside is the bare leaf;
		// when it's a deeper structure, wrap with the active child's name.
		if nextNode.typ == NodeAtomic || nextNode.typ == NodeFinal || nextNode.typ == NodeHistory {
			return AtomicValue(nextName)
		}
		return StateValue{Children: map[string]StateValue{nextName: newChildEntry}}
	}
}

// extractValueAt returns the value-inside `target` in `value`. For a target
// at the root (path empty), returns the value as-is. Returns (zero, false)
// if the path is not present in the value.
//
// The "value-inside" of a node is the same shape commitDescend computes for
// it — i.e. AtomicValue(leafName) when the active descendant is atomic,
// otherwise a {childName: nested} map. This is the canonical shape to feed
// back to spliceValueAt to restore the subtree.
func extractValueAt[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	value StateValue,
	target *stateNode[Ctx, Evt],
) (StateValue, bool) {
	if target == nil || target == root {
		return value, true
	}
	cursor := value
	for _, segment := range target.path {
		if cursor.IsAtomic() {
			if cursor.Leaf == segment {
				return AtomicValue(cursor.Leaf), true
			}
			return StateValue{}, false
		}
		child, ok := cursor.Children[segment]
		if !ok {
			return StateValue{}, false
		}
		cursor = child
	}
	return cursor, true
}

// spliceValueAt returns a new StateValue identical to `value` except that
// the value-inside `target` is replaced with `newInside`. Used by deep
// history to restore a saved subtree under a parent compound after
// commitValue has expanded its initial chain.
//
// Preserves parallel-region siblings outside the spliced path. If `target`
// is at the root (path empty), returns `newInside`. If the path does not
// exist in `value`, returns `value` unchanged (defensive — should not happen
// in practice because the parent compound was just (re-)entered).
func spliceValueAt[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	value StateValue,
	target *stateNode[Ctx, Evt],
	newInside StateValue,
) StateValue {
	if target == nil || target == root {
		return newInside
	}
	return spliceDescend(value, target.path, newInside)
}

// spliceDescend walks the path inside `current` and returns a new StateValue
// with `replacement` at the position. Siblings (in compound or parallel
// parents) are preserved. The function assumes `current` reflects the value
// rooted at `parent` in the caller (i.e., the value-inside the synthetic
// root for path[0], the value-inside path[0]'s node for path[1], etc.).
func spliceDescend(current StateValue, pathRemaining []string, replacement StateValue) StateValue {
	if len(pathRemaining) == 0 {
		return replacement
	}
	head := pathRemaining[0]
	rest := pathRemaining[1:]
	// Atomic-shaped current: only relevant when this is the final hop AND
	// the atomic Leaf is `head`. In that case, the whole current IS the
	// position to replace.
	if current.IsAtomic() {
		if current.Leaf != head {
			return current
		}
		if len(rest) == 0 {
			return replacement
		}
		// Should not happen for well-formed paths: descending past an
		// atomic. Defensive: leave unchanged.
		return current
	}
	// Compound or parallel: rebuild Children map, replacing only the
	// matching entry.
	if _, present := current.Children[head]; !present {
		return current
	}
	newChildren := make(map[string]StateValue, len(current.Children))
	for k, v := range current.Children {
		if k == head {
			if len(rest) == 0 {
				newChildren[k] = replacement
			} else {
				newChildren[k] = spliceDescend(v, rest, replacement)
			}
		} else {
			newChildren[k] = v
		}
	}
	return StateValue{Children: newChildren}
}
