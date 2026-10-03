package render

import (
	"sort"
	"strings"

	"github.com/arisros/fate/describe"
)

func sortedStateKeys(m map[string]describe.StateNodeDescriptor) []string {
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

func nodeID(path string) string {
	r := strings.NewReplacer(".", "_", "-", "_", " ", "_", "|", "_")
	return "s_" + r.Replace(path)
}

type descriptorIndex map[string]describe.StateNodeDescriptor

func indexDescriptor(d describe.MachineDescriptor) descriptorIndex {
	idx := descriptorIndex{}
	var walk func(prefix string, states map[string]describe.StateNodeDescriptor)
	walk = func(prefix string, states map[string]describe.StateNodeDescriptor) {
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
