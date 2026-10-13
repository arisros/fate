package describe

// SourceLocation is where one piece of a machine is written in Go source.
type SourceLocation struct {
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	Func string `json:"func,omitempty"`
	Path string `json:"path,omitempty"`
}

// SourceBuild identifies the build a set of locations belongs to, so a tool can turn a location into a link at an exact revision.
type SourceBuild struct {
	Module   string `json:"module"`
	Version  string `json:"version,omitempty"`
	Revision string `json:"revision,omitempty"`
	Modified bool   `json:"modified,omitempty"`
}

// MachineSources locates a machine and its guards and actions in Go source.
type MachineSources struct {
	Build   *SourceBuild            `json:"build,omitempty"`
	Machine *SourceLocation         `json:"machine,omitempty"`
	States  map[string]StateSources `json:"states,omitempty"`
}

// StateSources locates a state's actions and transitions.
type StateSources struct {
	Entry  []*SourceLocation              `json:"entry,omitempty"`
	Exit   []*SourceLocation              `json:"exit,omitempty"`
	On     map[string][]TransitionSources `json:"on,omitempty"`
	OnDone []TransitionSources            `json:"on_done,omitempty"`
	After  map[string][]TransitionSources `json:"after,omitempty"`
	Invoke []InvokeSources                `json:"invoke,omitempty"`
	Output *SourceLocation                `json:"output,omitempty"`
}

// InvokeSources locates the funcs of one invocation.
type InvokeSources struct {
	ID      string          `json:"id"`
	Input   *SourceLocation `json:"input,omitempty"`
	OnDone  *SourceLocation `json:"on_done,omitempty"`
	OnError *SourceLocation `json:"on_error,omitempty"`
}

// TransitionSources locates one transition's guard and actions.
type TransitionSources struct {
	Guard   *SourceLocation   `json:"guard,omitempty"`
	Actions []*SourceLocation `json:"actions,omitempty"`
}
