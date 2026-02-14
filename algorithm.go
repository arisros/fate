package fate

func lcca[Ctx any, Evt any](source, target *stateNode[Ctx, Evt], internal bool) *stateNode[Ctx, Evt] {
	if internal && isDescendant(target, source) {
		return source
	}
	ancestors := ancestorSet(source)
	for cursor := target.parent; cursor != nil; cursor = cursor.parent {
		if _, ok := ancestors[cursor]; ok {
			return cursor
		}
	}
	return rootOf(source)
}

func computeExitSet[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	current StateValue,
	source, target *stateNode[Ctx, Evt],
	internal bool,
) []*stateNode[Ctx, Evt] {
	leaf := resolveLeaf[Ctx, Evt](root, current)
	if leaf == nil {
		return nil
	}
	common := lcca[Ctx, Evt](source, target, internal)

	var exit []*stateNode[Ctx, Evt]
	for cursor := leaf; cursor != nil && cursor != common; cursor = cursor.parent {
		exit = append(exit, cursor)
	}
	return exit
}

func computeEntrySet[Ctx any, Evt any](
	source, target *stateNode[Ctx, Evt],
	internal bool,
) []*stateNode[Ctx, Evt] {
	common := lcca[Ctx, Evt](source, target, internal)

	chain := []*stateNode[Ctx, Evt]{}
	for cursor := target; cursor != nil && cursor != common; cursor = cursor.parent {
		chain = append([]*stateNode[Ctx, Evt]{cursor}, chain...)
	}

	cursor := target
	for cursor.typ == NodeCompound && cursor.name != "" {
		next, ok := cursor.children[cursor.initial]
		if !ok {
			break
		}
		chain = append(chain, next)
		cursor = next
	}

	return chain
}

func ancestorSet[Ctx any, Evt any](n *stateNode[Ctx, Evt]) map[*stateNode[Ctx, Evt]]struct{} {
	out := map[*stateNode[Ctx, Evt]]struct{}{}
	for cursor := n; cursor != nil; cursor = cursor.parent {
		out[cursor] = struct{}{}
	}
	return out
}

func isDescendant[Ctx any, Evt any](n, root *stateNode[Ctx, Evt]) bool {
	for cursor := n; cursor != nil; cursor = cursor.parent {
		if cursor == root {
			return true
		}
	}
	return false
}

func rootOf[Ctx any, Evt any](n *stateNode[Ctx, Evt]) *stateNode[Ctx, Evt] {
	for n.parent != nil {
		n = n.parent
	}
	return n
}
