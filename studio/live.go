// Package studio is a reusable, embeddable HTTP statechart studio — an
// XState-Studio-style viewer and live simulator for github.com/arisros/fate.
// Any program can build its own studio server, register its machines (static
// descriptors and/or live actors), and serve an interactive simulator.
//
// The fate-studio binary registers a set of demo machines; an application
// registers its own production machines by supplying a LiveInstance backed by
// the real machine.
package studio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	sc "github.com/arisros/fate"
)

// LiveInstance is a type-erased statechart actor the simulator drives
// without knowing the concrete Ctx / Evt type parameters. Build one with
// NewLiveActor.
type LiveInstance interface {
	Start(ctx context.Context) error
	// SendEvent dispatches the named event. Returns an error if the event
	// name is not recognised.
	SendEvent(ctx context.Context, eventName string) error
	// Snapshot returns the current state for SSE delivery.
	Snapshot() LiveSnapshot
	// Persist serialises the actor state for export.
	Persist() ([]byte, error)
	// Restore replaces the actor state from a persisted snapshot (undo/import).
	Restore(snapshot []byte) error
	// AvailableEvents lists the event names sendable from the active state.
	AvailableEvents() []string

	// PendingTimers lists the delayed ("after") transitions currently armed.
	PendingTimers() []TimerInfo
	// FireTimer delivers an elapsed delay for the given timer id.
	FireTimer(id string) error
	// PendingInvocations lists the invocations currently awaiting a result.
	PendingInvocations() []InvokeInfo
	// ResolveInvocation completes an invocation with the given JSON output
	// (empty string = null output).
	ResolveInvocation(id, outputJSON string) error
	// RejectInvocation fails an invocation with the given error message.
	RejectInvocation(id, errMsg string) error
}

// TimerInfo describes one armed delayed transition for the studio UI.
type TimerInfo struct {
	ID    string `json:"id"`
	Delay string `json:"delay"`
}

// InvokeInfo describes one pending invocation for the studio UI.
type InvokeInfo struct {
	ID  string `json:"id"`
	Src string `json:"src"`
}

// LiveSnapshot is the JSON payload pushed over SSE after every event. The
// graph STRUCTURE is fetched once from /m/{name}/graph; the snapshot only
// carries what changes per event (active path, context, status). The studio
// re-highlights the already-laid-out canvas — no re-layout per event.
type LiveSnapshot struct {
	Path        string          `json:"path"`
	Context     json.RawMessage `json:"context"`
	Status      sc.ActorStatus  `json:"status"`
	ASCII       string          `json:"ascii"` // ASCII diagram (CLI / static view)
	Timers      []TimerInfo     `json:"timers,omitempty"`
	Invocations []InvokeInfo    `json:"invocations,omitempty"`
}

// liveActor wraps a typed Actor[Ctx, Evt] as a LiveInstance.
type liveActor[Ctx any, Evt any] struct {
	machine  *sc.Machine[Ctx, Evt]
	actor    *sc.Actor[Ctx, Evt]
	dispatch func(name string) (Evt, error)
	describe func() sc.MachineDescriptor
}

// NewLiveActor builds a LiveInstance from a machine, an event-name
// dispatcher, and a descriptor function (for diagram rendering). The actor
// is created in Stopped status; the studio calls Start on first use.
//
//   - dispatch maps an event-name string to a typed event, or returns
//     ErrUnknownEvent for unrecognised names.
//   - describe returns the machine's MachineDescriptor (usually
//     machine.Describe()) — used to render the highlighted ASCII diagram
//     and to enumerate available events at the active state.
func NewLiveActor[Ctx any, Evt any](
	m *sc.Machine[Ctx, Evt],
	dispatch func(name string) (Evt, error),
	describe func() sc.MachineDescriptor,
) LiveInstance {
	return &liveActor[Ctx, Evt]{
		machine:  m,
		actor:    sc.NewActor(m),
		dispatch: dispatch,
		describe: describe,
	}
}

func (e *liveActor[Ctx, Evt]) Start(ctx context.Context) error {
	return e.actor.Start(ctx)
}

// Restore rebuilds the actor from a persisted snapshot (used by /undo and
// /import). The machine is re-used; only the actor instance is replaced.
func (e *liveActor[Ctx, Evt]) Restore(snapshot []byte) error {
	a, err := sc.NewActorFromSnapshot(e.machine, snapshot)
	if err != nil {
		return err
	}
	e.actor = a
	return nil
}

func (e *liveActor[Ctx, Evt]) SendEvent(_ context.Context, name string) error {
	evt, err := e.dispatch(name)
	if err != nil {
		return err
	}
	return e.actor.Send(context.Background(), evt)
}

func (e *liveActor[Ctx, Evt]) Snapshot() LiveSnapshot {
	snap := e.actor.Snapshot()
	ctxBytes, _ := json.Marshal(snap.Context)
	d := e.describe()
	activePath := snap.Value.Path()
	hl := highlightForActivePath(activePath)
	return LiveSnapshot{
		Path:        activePath,
		Context:     ctxBytes,
		Status:      snap.Status,
		ASCII:       sc.RenderASCII(d, sc.RenderOptions{Highlight: hl}),
		Timers:      e.PendingTimers(),
		Invocations: e.PendingInvocations(),
	}
}

func (e *liveActor[Ctx, Evt]) PendingTimers() []TimerInfo {
	pts := e.actor.PendingTimers()
	out := make([]TimerInfo, 0, len(pts))
	for _, p := range pts {
		out = append(out, TimerInfo{ID: string(p.ID), Delay: p.Delay.String()})
	}
	return out
}

func (e *liveActor[Ctx, Evt]) FireTimer(id string) error {
	e.actor.FireTimer(sc.TimerID(id))
	return nil
}

func (e *liveActor[Ctx, Evt]) PendingInvocations() []InvokeInfo {
	pis := e.actor.PendingInvocations()
	out := make([]InvokeInfo, 0, len(pis))
	for _, p := range pis {
		out = append(out, InvokeInfo{ID: string(p.ID), Src: p.Src})
	}
	return out
}

func (e *liveActor[Ctx, Evt]) ResolveInvocation(id, outputJSON string) error {
	var out interface{}
	if s := strings.TrimSpace(outputJSON); s != "" {
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return fmt.Errorf("output is not valid JSON: %w", err)
		}
	}
	e.actor.ResolveInvocation(sc.InvokeID(id), out)
	return nil
}

func (e *liveActor[Ctx, Evt]) RejectInvocation(id, errMsg string) error {
	if errMsg == "" {
		errMsg = "rejected from studio"
	}
	e.actor.RejectInvocation(sc.InvokeID(id), errors.New(errMsg))
	return nil
}

func (e *liveActor[Ctx, Evt]) Persist() ([]byte, error) {
	return e.actor.Persist()
}

func (e *liveActor[Ctx, Evt]) AvailableEvents() []string {
	d := e.describe()
	path := e.actor.Snapshot().Value.Path()
	seen := map[string]struct{}{}
	var evts []string
	// Parallel paths look like "a.x | b.y"; gather events from each region.
	for _, region := range strings.Split(path, " | ") {
		node, ok := descriptorNodeAt(d, strings.TrimSpace(region))
		if !ok {
			continue
		}
		for k := range node.On {
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			evts = append(evts, k)
		}
	}
	sort.Strings(evts)
	return evts
}

// ErrUnknownEvent is the canonical error a dispatch func returns for an
// unrecognised event name. The studio surfaces it as HTTP 400.
type ErrUnknownEvent struct{ Name string }

func (e ErrUnknownEvent) Error() string { return fmt.Sprintf("unknown event %q", e.Name) }

// highlightForActivePath highlights every active leaf (handles parallel).
func highlightForActivePath(path string) map[string]rune {
	if path == "" {
		return nil
	}
	h := map[string]rune{}
	for _, region := range strings.Split(path, " | ") {
		h[strings.TrimSpace(region)] = '▶'
	}
	return h
}

// descriptorNodeAt walks a MachineDescriptor by dot-path. Local copy of the
// engine's unexported lookupDescriptorPath.
func descriptorNodeAt(d sc.MachineDescriptor, path string) (sc.StateNodeDescriptor, bool) {
	if path == "" {
		return sc.StateNodeDescriptor{}, false
	}
	segs := strings.Split(path, ".")
	cur, ok := d.States[segs[0]]
	if !ok {
		return sc.StateNodeDescriptor{}, false
	}
	for _, s := range segs[1:] {
		next, ok := cur.States[s]
		if !ok {
			return sc.StateNodeDescriptor{}, false
		}
		cur = next
	}
	return cur, true
}
