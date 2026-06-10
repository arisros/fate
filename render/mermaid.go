package render

// Mermaid exporter — converts a MachineDescriptor to Mermaid stateDiagram-v2
// syntax for rendering a real node/edge graph. Pure-Go, stdlib-only (string
// building); the studio embeds mermaid.min.js to render the output client-side.

import (
	"sort"
	"strings"

	"github.com/arisros/fate"
)

// MermaidOptions controls the emitted diagram. Zero value renders top-to-bottom
// with no highlight.
type MermaidOptions struct {
	// Highlight maps active dot-paths to a marker (only the keys are used).
	// Each active leaf and its ancestor composites get the `active` class.
	Highlight map[string]rune

	// Direction is the Mermaid layout direction: "TB" (default), "LR", etc.
	Direction string
}

func (o *MermaidOptions) direction() string {
	if o.Direction == "" {
		return "TB"
	}
	return o.Direction
}

// Mermaid produces a Mermaid `stateDiagram-v2` document for the machine.
//
// Node IDs are the sanitised dot-path (so e.g. main.done and head_vd.done are
// distinct IDs `main_done` / `head_vd_done`, avoiding the collisions plain leaf
// names would cause). The human label keeps the leaf name. Compound and
// parallel states nest; parallel regions are divided by `--`.
func Mermaid(d fate.MachineDescriptor, opts MermaidOptions) string {
	var sb strings.Builder
	sb.WriteString("stateDiagram-v2\n")
	sb.WriteString("    direction " + opts.direction() + "\n")

	idx := indexDescriptor(d)

	if d.Initial != "" {
		sb.WriteString("    [*] --> " + nodeID(d.Initial) + "\n")
	}

	keys := sortedStateKeys(d.States)
	for _, k := range keys {
		emitMermaidNode(&sb, k, d.States[k], k, 1)
	}

	var edges []string
	for _, k := range keys {
		collectMermaidEdges(&edges, k, d.States[k], k, idx)
	}
	sort.Strings(edges)
	for _, e := range edges {
		sb.WriteString("    " + e + "\n")
	}

	emitMermaidClasses(&sb, d, idx, opts.Highlight)
	return sb.String()
}

func emitMermaidNode(sb *strings.Builder, name string, node fate.StateNodeDescriptor, path string, depth int) {
	indent := strings.Repeat("    ", depth)
	id := nodeID(path)
	label := mermaidLabel(name)

	switch node.Type {
	case "compound":
		sb.WriteString(indent + "state " + label + " as " + id + " {\n")
		if node.Initial != "" {
			sb.WriteString(indent + "    [*] --> " + nodeID(joinDotPath(path, node.Initial)) + "\n")
		}
		for _, k := range sortedStateKeys(node.States) {
			emitMermaidNode(sb, k, node.States[k], joinDotPath(path, k), depth+1)
		}
		sb.WriteString(indent + "}\n")

	case "parallel":
		sb.WriteString(indent + "state " + label + " as " + id + " {\n")
		regionKeys := sortedStateKeys(node.States)
		for i, k := range regionKeys {
			if i > 0 {
				sb.WriteString(indent + "    --\n")
			}
			emitMermaidNode(sb, k, node.States[k], joinDotPath(path, k), depth+1)
		}
		sb.WriteString(indent + "}\n")

	default:
		sb.WriteString(indent + "state " + label + " as " + id + "\n")
	}
}

func collectMermaidEdges(out *[]string, name string, node fate.StateNodeDescriptor, path string, idx descriptorIndex) {
	srcID := nodeID(path)

	events := make([]string, 0, len(node.On))
	for ev := range node.On {
		events = append(events, ev)
	}
	sort.Strings(events)
	for _, ev := range events {
		for _, t := range node.On[ev] {
			*out = append(*out, mermaidEdge(srcID, path, ev, t, idx))
		}
	}
	for _, t := range node.OnDone {
		*out = append(*out, mermaidEdge(srcID, path, "onDone", t, idx))
	}

	for _, k := range sortedStateKeys(node.States) {
		collectMermaidEdges(out, k, node.States[k], joinDotPath(path, k), idx)
	}
}

func mermaidEdge(srcID, srcPath, event string, t fate.TransitionDescriptor, idx descriptorIndex) string {
	tgtPath := resolveDescriptorTarget(srcPath, t.Target, idx)
	tgtID := nodeID(tgtPath)

	label := event
	if t.Guard != "" {
		label += " [" + t.Guard + "]"
	}
	if len(t.Actions) > 0 {
		named := make([]string, 0, len(t.Actions))
		for _, a := range t.Actions {
			if a != "" {
				named = append(named, a)
			}
		}
		if len(named) > 0 {
			label += " / " + strings.Join(named, ",")
		}
	}
	if t.Internal {
		label += " (internal)"
	}
	return srcID + " --> " + tgtID + " : " + mermaidEscapeLabel(label)
}

func emitMermaidClasses(sb *strings.Builder, d fate.MachineDescriptor, idx descriptorIndex, highlight map[string]rune) {
	var finals, histories []string
	for path, node := range idx {
		switch node.Type {
		case "final":
			finals = append(finals, nodeID(path))
		case "history":
			histories = append(histories, nodeID(path))
		}
	}
	sort.Strings(finals)
	sort.Strings(histories)
	if len(finals) > 0 {
		sb.WriteString("    classDef final fill:#eee,stroke:#888,stroke-width:2px,stroke-dasharray:3 2\n")
		sb.WriteString("    class " + strings.Join(finals, ",") + " final\n")
	}
	if len(histories) > 0 {
		sb.WriteString("    classDef history fill:#fff3cd,stroke:#b8860b\n")
		sb.WriteString("    class " + strings.Join(histories, ",") + " history\n")
	}

	if len(highlight) > 0 {
		active := map[string]struct{}{}
		for path := range highlight {
			parts := strings.Split(path, ".")
			for i := 1; i <= len(parts); i++ {
				active[strings.Join(parts[:i], ".")] = struct{}{}
			}
		}
		ids := make([]string, 0, len(active))
		for p := range active {
			if _, ok := idx[p]; ok {
				ids = append(ids, nodeID(p))
			}
		}
		sort.Strings(ids)
		if len(ids) > 0 {
			sb.WriteString("    classDef active fill:#dafbe1,stroke:#1a7f37,stroke-width:3px,font-weight:bold\n")
			sb.WriteString("    class " + strings.Join(ids, ",") + " active\n")
		}
	}
}

func mermaidLabel(name string) string {
	return "\"" + strings.ReplaceAll(name, "\"", "'") + "\""
}

func mermaidEscapeLabel(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, ":", "∶")
	return s
}
