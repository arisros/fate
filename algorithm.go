package fate

import (
	"maps"
	"slices"
	"sort"
	"strings"
)

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
	common := exitDomain[Ctx, Evt](lcca[Ctx, Evt](source, target, internal), target)

	var exit []*stateNode[Ctx, Evt]
	seen := map[*stateNode[Ctx, Evt]]bool{}
	for _, leaf := range resolveLeaves[Ctx, Evt](root, current) {
		if !isDescendant(leaf, common) {
			continue
		}
		for cursor := leaf; cursor != nil && cursor != common; cursor = cursor.parent {
			if seen[cursor] {
				break
			}
			seen[cursor] = true
			exit = append(exit, cursor)
		}
	}

	sort.SliceStable(exit, func(i, j int) bool {
		di, dj := len(exit[i].path), len(exit[j].path)
		if di != dj {
			return di > dj
		}
		return strings.Join(exit[i].path, ".") < strings.Join(exit[j].path, ".")
	})
	return exit
}

// Stops at the target's region so a parallel LCCA does not exit regions that stay active.
func exitDomain[Ctx any, Evt any](common, target *stateNode[Ctx, Evt]) *stateNode[Ctx, Evt] {
	if common == nil || common.typ != NodeParallel {
		return common
	}
	for cursor := target; cursor != nil; cursor = cursor.parent {
		if cursor.parent == common {
			return cursor
		}
	}
	return common
}

func computeEntrySet[Ctx any, Evt any](
	source, target *stateNode[Ctx, Evt],
	internal bool,
) []*stateNode[Ctx, Evt] {
	common := exitDomain[Ctx, Evt](lcca[Ctx, Evt](source, target, internal), target)

	chain := []*stateNode[Ctx, Evt]{}
	for cursor := target; cursor != nil && cursor != common; cursor = cursor.parent {
		chain = append([]*stateNode[Ctx, Evt]{cursor}, chain...)
	}

	entry := make([]*stateNode[Ctx, Evt], 0, len(chain))
	for i, n := range chain {
		entry = append(entry, n)
		var onChain *stateNode[Ctx, Evt]
		if i+1 < len(chain) {
			onChain = chain[i+1]
		}
		entry = append(entry, enterBelow(n, onChain)...)
	}
	return entry
}

func enterBelow[Ctx any, Evt any](n, onChain *stateNode[Ctx, Evt]) []*stateNode[Ctx, Evt] {
	switch n.typ {
	case NodeParallel:
		var out []*stateNode[Ctx, Evt]
		for _, name := range slices.Sorted(maps.Keys(n.children)) {
			region := n.children[name]
			if region == onChain {
				continue
			}
			out = append(out, region)
			out = append(out, enterBelow(region, nil)...)
		}
		return out
	case NodeCompound:
		if onChain != nil || n.name == "" {
			return nil
		}
		child, ok := n.children[n.initial]
		if !ok {
			return nil
		}
		return append([]*stateNode[Ctx, Evt]{child}, enterBelow(child, nil)...)
	}
	return nil
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
