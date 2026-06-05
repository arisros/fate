package fate

// ASCII graph rendering for a MachineDescriptor. Pure-Go, no TUI deps.
// Used by the P7 studio's machine_view and by ad-hoc debugging tools.
//
// Layout is deterministic, hierarchical, and small-machine-oriented:
//   - Each compound is rendered as a nested block, header line + indented
//     children block + footer line.
//   - Parallel regions appear stacked top-to-bottom, separated by `~~~`
//     dividers (renderers can swap to columns later; vertical stacking
//     keeps the renderer simple and avoids width-budgeting).
//   - Transitions for the cursor state are emitted in a sidebar block by
//     RenderTransitions; the main graph keeps states only.
//
// The renderer takes optional highlight info — a map of dot-paths to a
// marker character — so the simulator view can show the current active
// configuration. Highlights default to a leading "▶ " prefix; renderers
// can supply their own.

import (
	"fmt"
	"sort"
	"strings"
)

// RenderOptions controls cosmetic aspects of ASCII rendering. Zero value
// renders deterministically with no highlight.
type RenderOptions struct {
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

func (o *RenderOptions) indentStep() int {
	if o.IndentStep <= 0 {
		return 2
	}
	return o.IndentStep
}
func (o *RenderOptions) open() string {
	if o.CompoundOpen == "" {
		return "┌─"
	}
	return o.CompoundOpen
}
func (o *RenderOptions) close() string {
	if o.CompoundClose == "" {
		return "└─"
	}
	return o.CompoundClose
}

// RenderASCII produces a multi-line ASCII rendering of the descriptor.
// The result is suitable for printing to a terminal, embedding in a
// fixed-width log, or feeding to the studio's machine_view buffer.
//
// State child order is alphabetical for determinism (matches the rest of
// the library; see ADR-002 / ADR-007). Initial states are tagged with a
// trailing "(initial)" annotation.
func RenderASCII(d MachineDescriptor, opts RenderOptions) string {
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

// RenderTransitions emits a sidebar block showing every transition out of
// the state at the given dot-path. Returns an empty string if the path is
// not found in the descriptor. Format:
//
//	<event> [guard:NAME]: → <target> {Internal} [actions: A1, A2]
//
// Multiple alternatives for the same event appear on consecutive lines.
func RenderTransitions(d MachineDescriptor, path string) string {
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

func renderNode(sb *strings.Builder, name string, node StateNodeDescriptor, depth int, ancestorPath string, parentInitial string, opts *RenderOptions) {
	indent := strings.Repeat(" ", depth*opts.indentStep())
	dotPath := joinDotPath(ancestorPath, name)
	marker := highlightMarker(opts.Highlight, dotPath)

	// Atomic / final / history leaves render as a single line.
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

	// Compound / parallel render as bracketed block.
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

func nodeTagList(node StateNodeDescriptor, isInitial bool) []string {
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

func sortedStateKeys(m map[string]StateNodeDescriptor) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func joinDotPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func highlightMarker(highlight map[string]rune, dotPath string) string {
	if len(highlight) == 0 {
		return ""
	}
	// Prefer exact match. Otherwise, dotPath is an ANCESTOR of the
	// highlight target when the highlight key begins with dotPath+"." —
	// the parent compound (or any ancestor) gets marked when any
	// descendant is active.
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

func lookupDescriptorPath(d MachineDescriptor, path string) (StateNodeDescriptor, bool) {
	if path == "" {
		return StateNodeDescriptor{}, false
	}
	segments := strings.Split(path, ".")
	cursor, ok := d.States[segments[0]]
	if !ok {
		return StateNodeDescriptor{}, false
	}
	for _, seg := range segments[1:] {
		next, ok := cursor.States[seg]
		if !ok {
			return StateNodeDescriptor{}, false
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

func internalSuffix(t TransitionDescriptor) string {
	if !t.Internal {
		return ""
	}
	return " {internal}"
}

func actionsSuffix(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	// Filter empty (anonymous) names — they would render as "[actions: , ]".
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

func targetOrInternal(t TransitionDescriptor) string {
	if t.Target == "" {
		return "(no target — actions only)"
	}
	return t.Target
}
