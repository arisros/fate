package fate

import "errors"

var (
	// ErrInvalidConfig is returned by CreateMachine when the supplied config
	// fails validation. The wrapped error gives the specific reason.
	ErrInvalidConfig = errors.New("statechart: invalid machine config")

	// ErrUnknownTarget is returned when a transition's Target string does not
	// resolve to any sibling, descendant, or ancestor state path.
	ErrUnknownTarget = errors.New("statechart: unknown transition target")

	// ErrNoInitial is returned when a compound state node lacks an Initial
	// field naming one of its children.
	ErrNoInitial = errors.New("statechart: compound state has no initial child")

	// ErrUnknownInitial is returned when an Initial field names a state that
	// is not among the node's children.
	ErrUnknownInitial = errors.New("statechart: initial state not found among children")

	// ErrDuplicateState is returned when two sibling states share a name.
	ErrDuplicateState = errors.New("statechart: duplicate sibling state name")

	// ErrInvalidNodeType is returned when a state node has a Type the current
	// skeleton does not yet support (e.g. NodeParallel, NodeHistory, NodeFinal
	// before P5).
	ErrInvalidNodeType = errors.New("statechart: state node type not supported in this build")

	// ErrActorNotStarted is returned by Send when the actor's Start has not
	// been called yet.
	ErrActorNotStarted = errors.New("statechart: actor not started")

	// ErrActorStopped is returned by Send when the actor has been Stopped.
	ErrActorStopped = errors.New("statechart: actor stopped")
)
