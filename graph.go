package fate

// Graph JSON — a resolved node/edge model for the studio's self-hosted
// Stately-style canvas. The browser lays this out with elkjs and renders
// state cards + edges; it does not need to re-resolve targets (done here).
//
// Reuses the descriptor index + target resolver from mermaid.go.

import "sort"

// GraphNode is one state in the graph. Hierarchy is expressed via Parent
// (the qualified id of the enclosing compound/parallel node, "" for top level).
type GraphNode struct {
	ID      string   `json:"id"`      // qualified node id (nodeID of dot-path)
	Label   string   `json:"label"`   // leaf name (display)
	Path    string   `json:"path"`    // dot-path (for active-state matching)
	Type    string   `json:"type"`    // atomic|compound|parallel|final|history
	Parent  string   `json:"parent"`  // parent qualified id, "" if top level
	Initial bool     `json:"initial"` // is its parent's initial child
	History string   `json:"history,omitempty"`
	Entry   []string `json:"entry,omitempty"`
	Exit    []string `json:"exit,omitempty"`
}

// GraphEdge is one transition. Source/Target are qualified node ids; Event is
// the triggering event (the studio anchors the edge to the source node's
// matching event row, Stately-style).
type GraphEdge struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"`
	Event    string   `json:"event"`
	Target   string   `json:"target"`
	Guard    string   `json:"guard,omitempty"`
	Actions  []string `json:"actions,omitempty"`
	Internal bool     `json:"internal,omitempty"`
}

// Graph is the full resolved structure for one machine.
type Graph struct {
	ID      string      `json:"id"`
	Initial string      `json:"initial"` // qualified id of the top-level initial
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
}

// RenderGraphJSON converts a MachineDescriptor into a resolved Graph.
func RenderGraphJSON(d MachineDescriptor) Graph {
	idx := indexDescriptor(d)
	g := Graph{ID: d.ID}
	if d.Initial != "" {
		g.Initial = nodeID(d.Initial)
	}

	var walk func(name string, node StateNodeDescriptor, path, parentID, parentInitial string)
	walk = func(name string, node StateNodeDescriptor, path, parentID, parentInitial string) {
		n := GraphNode{
			ID:      nodeID(path),
			Label:   name,
			Path:    path,
			Type:    node.Type,
			Parent:  parentID,
			Initial: name == parentInitial,
			History: node.History,
			Entry:   node.Entry,
			Exit:    node.Exit,
		}
		g.Nodes = append(g.Nodes, n)

		// Edges out of this node (On + OnDone).
		events := make([]string, 0, len(node.On))
		for ev := range node.On {
			events = append(events, ev)
		}
		sort.Strings(events)
		ei := 0
		for _, ev := range events {
			for _, t := range node.On[ev] {
				g.Edges = append(g.Edges, edgeFor(path, ev, t, idx, &ei))
			}
		}
		for _, t := range node.OnDone {
			g.Edges = append(g.Edges, edgeFor(path, "onDone", t, idx, &ei))
		}

		// Recurse into children.
		for _, k := range sortedStateKeys(node.States) {
			walk(k, node.States[k], joinDotPath(path, k), n.ID, node.Initial)
		}
	}

	for _, k := range sortedStateKeys(d.States) {
		walk(k, d.States[k], k, "", d.Initial)
	}
	return g
}

func edgeFor(srcPath, event string, t TransitionDescriptor, idx descriptorIndex, ei *int) GraphEdge {
	tgtPath := resolveDescriptorTarget(srcPath, t.Target, idx)
	*ei++
	return GraphEdge{
		ID:       nodeID(srcPath) + "__" + event + "__" + itoa(*ei),
		Source:   nodeID(srcPath),
		Event:    event,
		Target:   nodeID(tgtPath),
		Guard:    t.Guard,
		Actions:  t.Actions,
		Internal: t.Internal,
	}
}

// itoa avoids importing strconv for one tiny use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
