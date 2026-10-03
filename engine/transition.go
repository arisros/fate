package engine

import "github.com/arisros/fate/persist"

// SelectedTransition records the outcome of selectTransitions per active leaf: the resolved source node and the matching transition config.
type SelectedTransition[Ctx any, Evt any] struct {
	Source *stateNode[Ctx, Evt]
	Config TransitionConfig[Ctx, Evt]
}

func selectTransitions[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	current persist.StateValue,
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

func resolveLeaf[Ctx any, Evt any](root *stateNode[Ctx, Evt], v persist.StateValue) *stateNode[Ctx, Evt] {
	leaves := resolveLeaves[Ctx, Evt](root, v)
	if len(leaves) == 0 {
		return nil
	}
	return leaves[0]
}

func resolveLeaves[Ctx any, Evt any](root *stateNode[Ctx, Evt], v persist.StateValue) []*stateNode[Ctx, Evt] {
	var out []*stateNode[Ctx, Evt]
	walkValue[Ctx, Evt](root, v, &out)
	return out
}

func walkValue[Ctx any, Evt any](parent *stateNode[Ctx, Evt], v persist.StateValue, out *[]*stateNode[Ctx, Evt]) {
	if parent.typ == NodeAtomic || parent.typ == NodeFinal || parent.typ == NodeHistory {
		*out = append(*out, parent)
		return
	}
	if v.IsAtomic() {
		child, ok := parent.children[v.Leaf]
		if !ok {
			return
		}
		switch child.typ {
		case NodeAtomic, NodeFinal, NodeHistory:
			*out = append(*out, child)
		case NodeCompound:
			if init, ok := child.children[child.initial]; ok {
				walkValue[Ctx, Evt](child, persist.AtomicValue(init.name), out)
			}
		case NodeParallel:
			walkValue[Ctx, Evt](child, child.initialInner(), out)
		}
		return
	}
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

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func commitValue[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	current persist.StateValue,
	target *stateNode[Ctx, Evt],
) persist.StateValue {
	if len(target.path) == 0 {
		return current
	}
	return commitDescend[Ctx, Evt](root, current, target.path, target)
}

func commitDescend[Ctx any, Evt any](
	parent *stateNode[Ctx, Evt],
	currentInside persist.StateValue,
	pathRemaining []string,
	target *stateNode[Ctx, Evt],
) persist.StateValue {
	if len(pathRemaining) == 0 {
		return target.initialInner()
	}
	nextName := pathRemaining[0]
	nextNode, ok := parent.children[nextName]
	if !ok {
		return currentInside
	}
	rest := pathRemaining[1:]

	var newChildEntry persist.StateValue
	if len(rest) == 0 {
		newChildEntry = nextNode.initialInner()
	} else {
		var nestedCurrent persist.StateValue
		if currentInside.Children != nil {
			nestedCurrent = currentInside.Children[nextName]
		}
		newChildEntry = commitDescend[Ctx, Evt](nextNode, nestedCurrent, rest, target)
	}

	switch parent.typ {
	case NodeParallel:
		regions := make(map[string]persist.StateValue, len(parent.children))
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
		return persist.StateValue{Children: regions}
	default:
		if nextNode.typ == NodeAtomic || nextNode.typ == NodeFinal || nextNode.typ == NodeHistory {
			return persist.AtomicValue(nextName)
		}
		return persist.StateValue{Children: map[string]persist.StateValue{nextName: newChildEntry}}
	}
}

func extractValueAt[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	value persist.StateValue,
	target *stateNode[Ctx, Evt],
) (persist.StateValue, bool) {
	if target == nil || target == root {
		return value, true
	}
	cursor := value
	for _, segment := range target.path {
		if cursor.IsAtomic() {
			if cursor.Leaf == segment {
				return persist.AtomicValue(cursor.Leaf), true
			}
			return persist.StateValue{}, false
		}
		child, ok := cursor.Children[segment]
		if !ok {
			return persist.StateValue{}, false
		}
		cursor = child
	}
	return cursor, true
}

func spliceValueAt[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	value persist.StateValue,
	target *stateNode[Ctx, Evt],
	newInside persist.StateValue,
) persist.StateValue {
	if target == nil || target == root {
		return newInside
	}
	return spliceDescend(value, target.path, newInside)
}

func spliceDescend(current persist.StateValue, pathRemaining []string, replacement persist.StateValue) persist.StateValue {
	if len(pathRemaining) == 0 {
		return replacement
	}
	head := pathRemaining[0]
	rest := pathRemaining[1:]
	if current.IsAtomic() {
		if current.Leaf != head {
			return current
		}
		if len(rest) == 0 {
			return replacement
		}
		return current
	}
	if _, present := current.Children[head]; !present {
		return current
	}
	newChildren := make(map[string]persist.StateValue, len(current.Children))
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
	return persist.StateValue{Children: newChildren}
}

func transitionPasses[Ctx any, Evt any](t TransitionConfig[Ctx, Evt], ctx Ctx, evt Evt, value persist.StateValue) bool {
	if t.Guard != nil && !t.Guard(ctx, evt) {
		return false
	}
	if t.Cond != nil && !t.Cond.Matches(value) {
		return false
	}
	return true
}
