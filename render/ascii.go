// Package render produces visual representations of a [fate.MachineDescriptor].
// All functions are pure (no I/O, no side effects) and deterministic.
//
// Three renderers are provided:
//   - [ASCII] — multi-line terminal diagram suitable for logging and CLIs.
//   - [Mermaid] — stateDiagram-v2 source for browser-side rendering.
//   - [GraphJSON] — resolved node/edge graph for canvas-based studio UIs.
//
// All renderers accept a [fate.MachineDescriptor], which is obtained from
// [fate.Machine.Describe]. They have no dependency on a live actor.
package render

// ASCII graph rendering for a MachineDescriptor. Pure-Go, no TUI deps.
//
// Layout is deterministic, hierarchical, and small-machine-oriented:
//   - Each compound is rendered as a nested block with header and footer lines.
//   - Parallel regions appear stacked top-to-bottom, separated by `~~~`
//     dividers.
//   - Transitions for the cursor state are emitted in a sidebar block by
//     [Transitions]; the main graph keeps states only.
//
// The renderer accepts optional highlight info — a map of dot-paths to a
// marker character — so a simulator view can show the current active
// configuration.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/arisros/fate"
)

// Options controls cosmetic aspects of ASCII rendering. Zero value renders
// deterministically with no highlight.
type Options struct {
	// Highlight maps dot-paths to a marker rune. The first match (longest
	// path) on each line determines the marker shown.
	Highlight map[string]rune

	// IndentStep is the per-level indent. Defaults to 2 spaces.
	IndentStep int

	// CompoundOpen / CompoundClose bracket compound state blocks. Defaults
	// to "┌─" / "└─" (Unicode box drawing).
	CompoundOpen  string
	CompoundClose string
}

func (o *Options) indentStep() int {
	if o.IndentStep <= 0 {
		return 2
	}
	return o.IndentStep
}
func (o *Options) open() string {
	if o.CompoundOpen == "" {
		return "┌─"
	}
	return o.CompoundOpen
}
func (o *Options) close() string {
	if o.CompoundClose == "" {
		return "└─"
	}
	return o.CompoundClose
}

// ASCII produces a multi-line ASCII rendering of the descriptor suitable for
// printing to a terminal, embedding in a fixed-width log, or feeding to a
// machine_view buffer.
//
// State child order is alphabetical for determinism (see ADR-002 / ADR-007).
// Initial states are tagged with a trailing "(initial)" annotation.
func ASCII(d fate.MachineDescriptor, opts Options) string {
	var sb strings.Builder
	header := fmt.Sprintf("Machine: %s (initial: %s)", d.ID, d.Initial)
	sb.WriteString(header)
	sb.WriteByte('\n')
	keys := sortedStateKeys(d.States)
	for _, k := range keys {
		renderNode(&sb, k, d.States[k], 0, "", d.Initial, &opts)
	}
	return sb.String()
}

// Transitions emits a sidebar block showing every transition out of the state
// at the given dot-path. Returns an empty string if the path is not found in
// the descriptor. Format:
//
//	<event> [guard:NAME]: → <target> {Internal} [actions: A1, A2]
//
// Multiple alternatives for the same event appear on consecutive lines.
func Transitions(d fate.MachineDescriptor, path string) string {
	node, ok := lookupDescriptorPath(d, path)
	if !ok {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Transitions from %s:\n", path)
	if len(node.On) == 0 && len(node.OnDone) == 0 {
		sb.WriteString("  (none)\n")
		return sb.String()
	}
	events := make([]string, 0, len(node.On))
	for k := range node.On {
		events = append(events, k)
	}
	sort.Strings(events)
	for _, ev := range events {
		for _, t := range node.On[ev] {
			fmt.Fprintf(&sb, "  %s%s: → %s%s%s\n",
				ev, guardSuffix(t.Guard), targetOrInternal(t),
				internalSuffix(t), actionsSuffix(t.Actions))
		}
	}
	for _, t := range node.OnDone {
		fmt.Fprintf(&sb, "  onDone%s: → %s%s%s\n",
			guardSuffix(t.Guard), targetOrInternal(t),
			internalSuffix(t), actionsSuffix(t.Actions))
	}
	return sb.String()
}

func renderNode(sb *strings.Builder, name string, node fate.StateNodeDescriptor, depth int, ancestorPath string, parentInitial string, opts *Options) {
	indent := strings.Repeat(" ", depth*opts.indentStep())
	dotPath := joinDotPath(ancestorPath, name)
	marker := highlightMarker(opts.Highlight, dotPath)

	leafTags := nodeTagList(node, name == parentInitial)
	if node.Type == "atomic" || node.Type == "final" || node.Type == "history" {
		sb.WriteString(indent)
		sb.WriteString(marker)
		sb.WriteString(name)
		if tag := strings.Join(leafTags, " "); tag != "" {
			sb.WriteString("  ")
			sb.WriteString(tag)
		}
		sb.WriteByte('\n')
		return
	}

	sb.WriteString(indent)
	sb.WriteString(marker)
	sb.WriteString(opts.open())
	sb.WriteString(" ")
	sb.WriteString(name)
	if tag := strings.Join(leafTags, " "); tag != "" {
		sb.WriteString("  ")
		sb.WriteString(tag)
	}
	sb.WriteByte('\n')

	childKeys := sortedStateKeys(node.States)
	for i, k := range childKeys {
		if node.Type == "parallel" && i > 0 {
			sb.WriteString(indent)
			sb.WriteString(strings.Repeat(" ", opts.indentStep()))
			sb.WriteString("~~~\n")
		}
		renderNode(sb, k, node.States[k], depth+1, dotPath, node.Initial, opts)
	}

	sb.WriteString(indent)
	sb.WriteString(opts.close())
	sb.WriteString(" ")
	sb.WriteString(name)
	sb.WriteByte('\n')
}

func nodeTagList(node fate.StateNodeDescriptor, isInitial bool) []string {
	var tags []string
	if isInitial {
		tags = append(tags, "(initial)")
	}
	switch node.Type {
	case "parallel":
		tags = append(tags, "[parallel]")
	case "final":
		tags = append(tags, "[final]")
	case "history":
		hist := node.History
		if hist == "" {
			hist = "shallow"
		}
		tags = append(tags, fmt.Sprintf("[history:%s]", hist))
		if node.Default != "" {
			tags = append(tags, fmt.Sprintf("default=%s", node.Default))
		}
	}
	return tags
}

func highlightMarker(highlight map[string]rune, dotPath string) string {
	if len(highlight) == 0 {
		return ""
	}
	if r, ok := highlight[dotPath]; ok {
		return string(r) + " "
	}
	for k, r := range highlight {
		if dotPath != "" && strings.HasPrefix(k, dotPath+".") {
			return string(r) + " "
		}
	}
	return ""
}

func lookupDescriptorPath(d fate.MachineDescriptor, path string) (fate.StateNodeDescriptor, bool) {
	if path == "" {
		return fate.StateNodeDescriptor{}, false
	}
	segments := strings.Split(path, ".")
	cursor, ok := d.States[segments[0]]
	if !ok {
		return fate.StateNodeDescriptor{}, false
	}
	for _, seg := range segments[1:] {
		next, ok := cursor.States[seg]
		if !ok {
			return fate.StateNodeDescriptor{}, false
		}
		cursor = next
	}
	return cursor, true
}

func guardSuffix(name string) string {
	if name == "" {
		return ""
	}
	return " [guard:" + name + "]"
}

func internalSuffix(t fate.TransitionDescriptor) string {
	if !t.Internal {
		return ""
	}
	return " {internal}"
}

func actionsSuffix(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	var named []string
	for _, a := range actions {
		if a != "" {
			named = append(named, a)
		}
	}
	if len(named) == 0 {
		return fmt.Sprintf(" [actions: <%d anonymous>]", len(actions))
	}
	return fmt.Sprintf(" [actions: %s]", strings.Join(named, ", "))
}

func targetOrInternal(t fate.TransitionDescriptor) string {
	if t.Target == "" {
		return "(no target — actions only)"
	}
	return t.Target
}
