package main

import (
	"github.com/arisros/fate"
	"github.com/arisros/fate/internal/demos"
)

type registryEntry struct {
	name    string
	summary string
	build   func() fate.MachineDescriptor
}

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
