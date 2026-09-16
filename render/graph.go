package render

import (
	"encoding/json"
	"sort"

	"github.com/arisros/fate"
)

// GraphNode is one state in the graph.
type GraphNode struct {
	ID            string          `json:"id"`
	Label         string          `json:"label"`
	Path          string          `json:"path"`
	Type          string          `json:"type"`
	Parent        string          `json:"parent"`
	Initial       bool            `json:"initial"`
	History       string          `json:"history,omitempty"`
	Entry         []string        `json:"entry,omitempty"`
	Exit          []string        `json:"exit,omitempty"`
	UIStateSchema json.RawMessage `json:"ui_state_schema,omitempty"`
}

// GraphEdge is one transition.
type GraphEdge struct {
	ID       string         `json:"id"`
	Source   string         `json:"source"`
	Event    string         `json:"event"`
	Target   string         `json:"target"`
	Guard    string         `json:"guard,omitempty"`
	Actions  []string       `json:"actions,omitempty"`
	Internal bool           `json:"internal,omitempty"`
	CondMeta *fate.CondMeta `json:"cond_meta,omitempty"`
}

// Graph is the full resolved structure for one machine.
type Graph struct {
	ID      string      `json:"id"`
	Initial string      `json:"initial"`
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
}

// GraphJSON converts a MachineDescriptor into a resolved Graph.
func GraphJSON(d fate.MachineDescriptor) Graph {
	idx := indexDescriptor(d)
	g := Graph{ID: d.ID}
	if d.Initial != "" {
		g.Initial = nodeID(d.Initial)
	}

	var walk func(name string, node fate.StateNodeDescriptor, path, parentID, parentInitial string)
	walk = func(name string, node fate.StateNodeDescriptor, path, parentID, parentInitial string) {
		n := GraphNode{
			ID:            nodeID(path),
			Label:         name,
			Path:          path,
			Type:          node.Type,
			Parent:        parentID,
			Initial:       name == parentInitial,
			History:       node.History,
			Entry:         node.Entry,
			Exit:          node.Exit,
			UIStateSchema: node.UIStateSchema,
		}
		g.Nodes = append(g.Nodes, n)

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

		for _, k := range sortedStateKeys(node.States) {
			walk(k, node.States[k], joinDotPath(path, k), n.ID, node.Initial)
		}
	}

	for _, k := range sortedStateKeys(d.States) {
		walk(k, d.States[k], k, "", d.Initial)
	}
	return g
}

func edgeFor(srcPath, event string, t fate.TransitionDescriptor, idx descriptorIndex, ei *int) GraphEdge {
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
		CondMeta: t.CondMeta,
	}
}

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
