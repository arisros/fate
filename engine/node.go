package engine

import "fmt"

// NodeType discriminates state node kinds.
type NodeType uint8

const (
	NodeAtomic NodeType = iota
	NodeCompound
	NodeParallel
	NodeFinal
	NodeHistory
)

// History selects the depth of memory for a NodeHistory pseudo-state.
type History uint8

const (
	HistoryShallow History = iota
	HistoryDeep
)

// String returns the textual name of the node type.
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
