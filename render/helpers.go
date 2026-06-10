package render

// Shared private helpers used across ascii.go, mermaid.go, and graph.go.
// None of these are exported; they exist only to avoid duplication within the
// render package.

import (
	"sort"
	"strings"

	"github.com/arisros/fate"
)

// sortedStateKeys returns the keys of a StateNodeDescriptor map sorted
// alphabetically. Determinism matches the rest of the library (ADR-002/007).
func sortedStateKeys(m map[string]fate.StateNodeDescriptor) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// joinDotPath joins a prefix and a name with a dot separator.
// Returns name unchanged when prefix is empty.
func joinDotPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// nodeID sanitises a dot-path into a Mermaid-safe identifier.
func nodeID(path string) string {
	r := strings.NewReplacer(".", "_", "-", "_", " ", "_", "|", "_")
	return "s_" + r.Replace(path)
}

// descriptorIndex is a flat map from dot-path to StateNodeDescriptor,
// built once and reused for O(1) target resolution.
type descriptorIndex map[string]fate.StateNodeDescriptor

func indexDescriptor(d fate.MachineDescriptor) descriptorIndex {
	idx := descriptorIndex{}
	var walk func(prefix string, states map[string]fate.StateNodeDescriptor)
	walk = func(prefix string, states map[string]fate.StateNodeDescriptor) {
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
// Returns the resolved dot-path, or the raw target (best-effort) when it
// can't be resolved — the studio still renders an edge in that case.
func resolveDescriptorTarget(srcPath, target string, idx descriptorIndex) string {
	if target == "" {
		return srcPath
	}
	if cand := joinDotPath(srcPath, target); pathExists(cand, idx) {
		return cand
	}
	parts := strings.Split(srcPath, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		ancestor := strings.Join(parts[:i], ".")
		if cand := joinDotPath(ancestor, target); pathExists(cand, idx) {
			return cand
		}
	}
	if pathExists(target, idx) {
		return target
	}
	return target
}

func pathExists(path string, idx descriptorIndex) bool {
	_, ok := idx[path]
	return ok
}
