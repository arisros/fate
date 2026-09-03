package fate

import (
	"maps"
	"slices"
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
	common := exitDomain[Ctx, Evt](lcca[Ctx, Evt](source, target, internal), target)

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

// exitDomain narrows an LCCA that is a parallel node down to the single region
// the transition actually relocates.
//
// The exit set has to agree with what commitValue does, and commitValue only
// ever replaces the target's own path: every sibling region of a parallel node
// on that path is carried over untouched. So when the LCCA is a parallel node,
// treating it as the domain would exit states that are still active afterwards
// — their Exit actions would run and their timers and invocations would be
// disarmed while the machine still reported them active, leaving an armed
// timer that can never fire. Descending to the target's own region keeps the
// two halves consistent.
//
// This is narrower than SCXML, which exits and re-enters every region.
// computeEntrySet uses this same domain, so a cross-region transition relocates
// only the target's region. Adopting SCXML means widening both halves together;
// widening one alone is what produced the inconsistency above.
func exitDomain[Ctx any, Evt any](common, target *stateNode[Ctx, Evt]) *stateNode[Ctx, Evt] {
	if common == nil || common.typ != NodeParallel {
		return common
	}
	// The child of common that contains (or is) target is the region being
	// replaced. A transition targeting the parallel node itself has no such
	// child, and keeps the parallel node as its domain.
	for cursor := target; cursor != nil; cursor = cursor.parent {
		if cursor.parent == common {
			return cursor
		}
	}
	return common
}

// computeEntrySet returns the nodes to enter, outermost first.
//
// The domain is exitDomain's rather than the bare LCCA so both halves share one
// boundary. A transition staying inside a parallel node re-enters no sibling,
// matching the fact that none of them exited; one whose domain is above the
// node re-enters every region, because every region exited. The bare LCCA would
// re-enter regions the exit set deliberately left running.
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

// enterBelow returns the descendants entered with n, where onChain is the child
// the caller's chain already names (nil when the chain ends at n). Regions are
// visited in sorted name order so arming does not depend on map iteration.
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
			return nil // CreateMachine rejects this config (ErrUnknownInitial)
		}
		return append([]*stateNode[Ctx, Evt]{child}, enterBelow(child, nil)...)
	}
	return nil
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
