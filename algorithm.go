package fate

import (
	"sort"
	"strings"
)

// SCXML transition algorithms.
//
// Adapted from the W3C SCXML spec and XState v5's `stateUtils.ts`. Only the
// pieces relevant to compound-hierarchy + atomic states land in P4; the
// parallel-region pieces (computeEntrySet for parallel ancestors, etc.)
// extend these in P5.
//
// Vocabulary:
//   - LCCA  (Least Common Compound Ancestor): the deepest compound state
//     that contains both the source and target states. For internal
//     transitions, the LCCA is the source state itself (so its exit set
//     is empty).
//   - Exit set: the ordered list of states left when firing a transition,
//     deepest first. Exit actions run in this order.
//   - Entry set: the ordered list of states entered when firing a transition,
//     outermost first (descending into target's initial chain). Entry actions
//     run in this order.

// lcca returns the least common compound ancestor of source and target. If
// the transition is internal AND target is a descendant of source, the LCCA
// is source itself. For all other cases, LCCA is the first compound ancestor
// shared by both nodes (the synthetic root is always shared, so the search
// terminates).
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
	// The synthetic root is in every node's ancestor chain; this is
	// unreachable in well-formed machines.
	return rootOf(source)
}

// computeExitSet returns the ordered list of nodes that should exit when
// firing the transition. Order: deepest first (so exit actions run
// child-then-parent).
//
// The exit set is every active node strictly below the LCCA. For a
// non-parallel configuration that is the single chain from the active leaf up
// to (but not including) the LCCA. For internal transitions where target is a
// descendant of source, source itself stays active and only its descendants on
// the active branch exit.
//
// Parallel regions are why this is stated in terms of the LCCA rather than
// "the active leaf". Several leaves are active at once, and the ones that
// matter are exactly those inside the transition's domain: a transition fired
// within one region has that region's ancestor as its LCCA, so no sibling
// region is below it and none of their states exit. A transition that leaves
// the parallel node itself has an LCCA above it, so every region's states
// exit, each contributing its own chain.
func computeExitSet[Ctx any, Evt any](
	root *stateNode[Ctx, Evt],
	current StateValue,
	source, target *stateNode[Ctx, Evt],
	internal bool,
) []*stateNode[Ctx, Evt] {
	common := lcca[Ctx, Evt](source, target, internal)

	var exit []*stateNode[Ctx, Evt]
	seen := map[*stateNode[Ctx, Evt]]bool{}
	for _, leaf := range resolveLeaves[Ctx, Evt](root, current) {
		// A leaf outside the domain belongs to a region the transition does
		// not touch. Walking it would run the wrong states' exit actions and
		// disarm their timers and invocations.
		if !isDescendant(leaf, common) {
			continue
		}
		for cursor := leaf; cursor != nil && cursor != common; cursor = cursor.parent {
			if seen[cursor] {
				break // this chain has merged into one already collected
			}
			seen[cursor] = true
			exit = append(exit, cursor)
		}
	}

	// Deepest first, so a child's exit action runs before its parent's. Depth
	// alone leaves siblings from different regions unordered, so path breaks
	// the tie and keeps the order independent of map iteration.
	sort.SliceStable(exit, func(i, j int) bool {
		di, dj := len(exit[i].path), len(exit[j].path)
		if di != dj {
			return di > dj
		}
		return strings.Join(exit[i].path, ".") < strings.Join(exit[j].path, ".")
	})
	return exit
}

// computeEntrySet returns the ordered list of nodes that should be entered.
// Order: outermost first (so entry actions run parent-then-child).
//
// The entry set is the chain from (the child of LCCA that is an ancestor of
// target) down to target, then target's initial descendants.
func computeEntrySet[Ctx any, Evt any](
	source, target *stateNode[Ctx, Evt],
	internal bool,
) []*stateNode[Ctx, Evt] {
	common := lcca[Ctx, Evt](source, target, internal)

	// Find the child of `common` that contains (or equals) `target`.
	chain := []*stateNode[Ctx, Evt]{}
	for cursor := target; cursor != nil && cursor != common; cursor = cursor.parent {
		chain = append([]*stateNode[Ctx, Evt]{cursor}, chain...)
	}

	// Descend into target's initial chain.
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

// ancestorSet returns the set of all ancestors of n, including n itself.
// Map used as a set for O(1) lookups during LCCA computation.
func ancestorSet[Ctx any, Evt any](n *stateNode[Ctx, Evt]) map[*stateNode[Ctx, Evt]]struct{} {
	out := map[*stateNode[Ctx, Evt]]struct{}{}
	for cursor := n; cursor != nil; cursor = cursor.parent {
		out[cursor] = struct{}{}
	}
	return out
}

// isDescendant reports whether n is a (strict or equal) descendant of root.
func isDescendant[Ctx any, Evt any](n, root *stateNode[Ctx, Evt]) bool {
	for cursor := n; cursor != nil; cursor = cursor.parent {
		if cursor == root {
			return true
		}
	}
	return false
}

// rootOf returns the synthetic root of n's tree.
func rootOf[Ctx any, Evt any](n *stateNode[Ctx, Evt]) *stateNode[Ctx, Evt] {
	for n.parent != nil {
		n = n.parent
	}
	return n
}

// (Earlier P4 helpers buildValueFromEntrySet / initialValueIfCompound
// were removed; commitValue in transition.go now handles value composition
// — including parallel-region sibling preservation — via the canonical
// stateNode.initialInner / initialValue methods.)
