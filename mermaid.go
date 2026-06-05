package fate

// Mermaid exporter — converts a MachineDescriptor to Mermaid stateDiagram-v2
// syntax for rendering a real node/edge graph in the studio UI. Pure-Go,
// stdlib-only (string building); the studio embeds mermaid.min.js to render
// the output client-side.
//
// Mirrors RenderASCII's shape (ascii_graph.go). The descriptor is a complete
// graph model, so this only emits syntax — Mermaid does the layout.

import (
	"sort"
	"strings"
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

// RenderMermaid produces a Mermaid `stateDiagram-v2` document for the machine.
//
// Node IDs are the sanitised dot-path (so e.g. main.done and head_vd.done are
// distinct IDs `main_done` / `head_vd_done`, avoiding the collisions plain leaf
// names would cause). The human label keeps the leaf name. Compound and
// parallel states nest; parallel regions are divided by `--`.
func RenderMermaid(d MachineDescriptor, opts MermaidOptions) string {
	var sb strings.Builder
	sb.WriteString("stateDiagram-v2\n")
	sb.WriteString("    direction " + opts.direction() + "\n")

	// Index every node by dot-path so transition targets resolve correctly.
	idx := indexDescriptor(d)

	// Root initial edge.
	if d.Initial != "" {
		sb.WriteString("    [*] --> " + nodeID(d.Initial) + "\n")
	}

	// Emit state blocks (structure), then transitions (edges), then classes.
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

// ----- structure emission -----

func emitMermaidNode(sb *strings.Builder, name string, node StateNodeDescriptor, path string, depth int) {
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
		// atomic / final / history — single node line. (Final & history get a
		// class via emitMermaidClasses; they still render as a normal node so
		// cross-composite edges resolve unambiguously.)
		sb.WriteString(indent + "state " + label + " as " + id + "\n")
	}
}

// ----- edge collection -----

func collectMermaidEdges(out *[]string, name string, node StateNodeDescriptor, path string, idx descriptorIndex) {
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

func mermaidEdge(srcID, srcPath, event string, t TransitionDescriptor, idx descriptorIndex) string {
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

// ----- classes (final / history / active highlight) -----

func emitMermaidClasses(sb *strings.Builder, d MachineDescriptor, idx descriptorIndex, highlight map[string]rune) {
	// Final + history styling.
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

	// Active highlight: each highlighted leaf + its ancestor composites.
	if len(highlight) > 0 {
		active := map[string]struct{}{}
		for path := range highlight {
			// Mark the leaf and every ancestor (so composites containing the
			// active leaf are also emphasised).
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

// ----- descriptor index + target resolution -----

type descriptorIndex map[string]StateNodeDescriptor

func indexDescriptor(d MachineDescriptor) descriptorIndex {
	idx := descriptorIndex{}
	var walk func(prefix string, states map[string]StateNodeDescriptor)
	walk = func(prefix string, states map[string]StateNodeDescriptor) {
		for name, node := range states {
			path := joinDotPath(prefix, name)
			idx[path] = node
			if len(node.States) > 0 {
				walk(path, node.States)
			}
		}
	}
	walk("", d.States)
	return idx
}

// resolveDescriptorTarget mirrors the engine's resolveTarget (machine.go):
//  1. a descendant of the source,
//  2. an ancestor's sibling (walking up),
//  3. an absolute path from the root.
//
// Returns the resolved dot-path, or the raw target (best-effort) when it can't
// be resolved — the studio still renders an edge in that case.
func resolveDescriptorTarget(srcPath, target string, idx descriptorIndex) string {
	if target == "" {
		return srcPath // internal / no-target — self
	}
	// 1) descendant of source.
	if cand := joinDotPath(srcPath, target); pathExists(cand, idx) {
		return cand
	}
	// 2) walk up ancestors, try sibling.
	parts := strings.Split(srcPath, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		ancestor := strings.Join(parts[:i], ".")
		if cand := joinDotPath(ancestor, target); pathExists(cand, idx) {
			return cand
		}
	}
	// 3) absolute from root.
	if pathExists(target, idx) {
		return target
	}
	return target
}

// pathExists reports whether the dot-path (possibly multi-segment) resolves to
// an indexed node.
func pathExists(path string, idx descriptorIndex) bool {
	_, ok := idx[path]
	return ok
}

// ----- id / label helpers -----

// nodeID sanitises a dot-path into a Mermaid-safe identifier.
func nodeID(path string) string {
	r := strings.NewReplacer(".", "_", "-", "_", " ", "_", "|", "_")
	return "s_" + r.Replace(path)
}

// mermaidLabel quotes the human label for `state "label" as id`.
func mermaidLabel(name string) string {
	return "\"" + strings.ReplaceAll(name, "\"", "'") + "\""
}

// mermaidEscapeLabel makes an edge label safe (no colons/newlines that would
// break Mermaid parsing). Event/guard/action names are identifiers, so this is
// defensive.
func mermaidEscapeLabel(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, ":", "∶") // ratio char — visually a colon
	return s
}
