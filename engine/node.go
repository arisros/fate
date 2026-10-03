package engine

import "fmt"

// NodeType discriminates state node kinds. As of P5, Atomic, Compound,
// Final, and History are supported; Parallel is the remaining P5 piece.
type NodeType uint8

const (
	NodeAtomic NodeType = iota
	NodeCompound
	NodeParallel // P5 follow-up
	NodeFinal
	NodeHistory
)

// History selects the depth of memory for a NodeHistory pseudo-state.
//
//   - HistoryShallow remembers only the immediate child of the parent compound.
//     On re-entry, the parent restarts that child via the child's initial chain.
//   - HistoryDeep remembers the full descendant configuration. On re-entry,
//     the entire active sub-tree at exit time is restored.
type History uint8

const (
	HistoryShallow History = iota
	HistoryDeep
)

// String returns the textual name of the node type. Used in error messages
// and snapshot debugging output.
func (t NodeType) String() string {
	switch t {
	case NodeAtomic:
		return "atomic"
	case NodeCompound:
		return "compound"
	case NodeParallel:
		return "parallel"
	case NodeFinal:
		return "final"
	case NodeHistory:
		return "history"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}
