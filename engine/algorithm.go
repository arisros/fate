package engine

import (
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/arisros/fate/persist"
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
	current persist.StateValue,
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
	restore *deepHistorySplice[Ctx, Evt],
) []*stateNode[Ctx, Evt] {
	common := exitDomain[Ctx, Evt](lcca[Ctx, Evt](source, target, internal), target)

	var chain []*stateNode[Ctx, Evt]
	for cursor := target; cursor != nil && cursor != common; cursor = cursor.parent {
		chain = append([]*stateNode[Ctx, Evt]{cursor}, chain...)
	}

	e := entryBuilder[Ctx, Evt]{saved: map[*stateNode[Ctx, Evt]]persist.StateValue{}}
	if restore != nil {
		e.record(restore.parent, restore.subtree)
	}
	if len(chain) == 0 {
		return e.content(target, nil)
	}
	return e.from(chain)
}

type entryBuilder[Ctx any, Evt any] struct {
	saved map[*stateNode[Ctx, Evt]]persist.StateValue
}

func (e entryBuilder[Ctx, Evt]) record(n *stateNode[Ctx, Evt], v persist.StateValue) {
	e.saved[n] = v
	if v.IsAtomic() {
		return
	}
	for name, child := range v.Children {
		if c, ok := n.children[name]; ok {
			e.record(c, child)
		}
	}
}

func (e entryBuilder[Ctx, Evt]) from(chain []*stateNode[Ctx, Evt]) []*stateNode[Ctx, Evt] {
	return append([]*stateNode[Ctx, Evt]{chain[0]}, e.content(chain[0], chain[1:])...)
}

func (e entryBuilder[Ctx, Evt]) content(n *stateNode[Ctx, Evt], rest []*stateNode[Ctx, Evt]) []*stateNode[Ctx, Evt] {
	switch n.typ {
	case NodeParallel:
		var out []*stateNode[Ctx, Evt]
		for _, name := range slices.Sorted(maps.Keys(n.children)) {
			region := n.children[name]
			if len(rest) > 0 && rest[0] == region {
				out = append(out, e.from(rest)...)
			} else {
				out = append(out, e.from([]*stateNode[Ctx, Evt]{region})...)
			}
		}
		return out
	case NodeCompound:
		if len(rest) > 0 {
			return e.from(rest)
		}
		if n.name == "" {
			return nil
		}
		child, ok := n.children[e.childToEnter(n)]
		if !ok {
			return nil
		}
		return e.from([]*stateNode[Ctx, Evt]{child})
	}
	return nil
}

func (e entryBuilder[Ctx, Evt]) childToEnter(n *stateNode[Ctx, Evt]) string {
	v, ok := e.saved[n]
	if !ok {
		return n.initial
	}
	if v.IsAtomic() {
		return v.Leaf
	}
	for name := range v.Children {
		return name
	}
	return n.initial
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
