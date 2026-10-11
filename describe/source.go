package describe

// SourceLocation is where one piece of a machine is written in Go source.
type SourceLocation struct {
	// File is the package import path joined with the file's base name, such as
	// "github.com/acme/app/orders/machine.go". It is the same on every host.
	File string `json:"file"`
	// Line is the 1-based line number in File.
	Line int `json:"line"`
	// Func is the Go function a guard or action runs. Empty for a machine.
	Func string `json:"func,omitempty"`
	// Path is the file as the compiler recorded it: absolute on the build host,
	// or module-relative when built with -trimpath.
	Path string `json:"path,omitempty"`
}

// SourceBuild identifies the build a set of locations belongs to, so a tool can
// turn a location into a link at an exact revision.
type SourceBuild struct {
	// Module is the main module's path.
	Module string `json:"module"`
	// Version is the main module's version, when the build carries one.
	Version string `json:"version,omitempty"`
	// Revision is the VCS commit the binary was built from, when stamped.
	Revision string `json:"revision,omitempty"`
	// Modified reports that the build had uncommitted changes.
	Modified bool `json:"modified,omitempty"`
}

// MachineSources locates a machine and its guards and actions in Go source.
// It is tooling data: nothing in the engine reads it back, and it is kept
// apart from MachineDescriptor so a descriptor stays independent of line
// numbers.
type MachineSources struct {
	// Build is nil when the binary carries no build information.
	Build *SourceBuild `json:"build,omitempty"`
	// Machine is where the machine was created.
	Machine *SourceLocation `json:"machine,omitempty"`
	// States is keyed by dot-separated state path and holds only the states
	// that have at least one located guard or action.
	States map[string]StateSources `json:"states,omitempty"`
}

// StateSources locates a state's actions and transitions. Every slice lines up
// with the slice of the same name in StateNodeDescriptor; an entry is nil when
// that element has no recoverable location.
type StateSources struct {
	Entry  []*SourceLocation              `json:"entry,omitempty"`
	Exit   []*SourceLocation              `json:"exit,omitempty"`
	On     map[string][]TransitionSources `json:"on,omitempty"`
	OnDone []TransitionSources            `json:"on_done,omitempty"`
}

// TransitionSources locates one transition's guard and actions. Actions lines
// up with TransitionDescriptor.Actions.
type TransitionSources struct {
	Guard   *SourceLocation   `json:"guard,omitempty"`
	Actions []*SourceLocation `json:"actions,omitempty"`
}
