package main

import (
	"github.com/arisros/fate"
	"github.com/arisros/fate/internal/demos"
)

// registryEntry pairs a machine name with a one-line summary and a builder that
// returns its descriptor for the data-layer commands.
type registryEntry struct {
	name    string
	summary string
	build   func() fate.MachineDescriptor
}

// registry is the built-in set of demo machines, sourced from internal/demos so
// the fate CLI and the fate-studio server stay in sync.
var registry = buildRegistry()

func buildRegistry() []registryEntry {
	all := demos.All()
	out := make([]registryEntry, 0, len(all))
	for _, d := range all {
		d := d
		out = append(out, registryEntry{
			name:    d.Name,
			summary: d.Summary,
			build:   d.Descriptor,
		})
	}
	return out
}
