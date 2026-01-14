// Package studio is a reusable, embeddable HTTP statechart studio — an XState-Studio-style viewer and live simulator for github.com/arisros/fate.
package studio

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sc "github.com/arisros/fate"
)

// LiveInstance is a type-erased statechart actor the simulator drives without knowing the concrete Ctx / Evt type parameters.
type LiveInstance interface {
	Start(ctx context.Context) error
	SendEvent(ctx context.Context, eventName string) error
	Snapshot() LiveSnapshot
	Persist() ([]byte, error)
	Restore(snapshot []byte) error
	AvailableEvents() []string
}

// LiveSnapshot is the JSON payload pushed over SSE after every event.
type LiveSnapshot struct {
	Path    string          `json:"path"`
	Context json.RawMessage `json:"context"`
	Status  sc.ActorStatus  `json:"status"`
	ASCII   string          `json:"ascii"`
}

type liveActor[Ctx any, Evt any] struct {
	machine  *sc.Machine[Ctx, Evt]
	actor    *sc.Actor[Ctx, Evt]
	dispatch func(name string) (Evt, error)
	describe func() sc.MachineDescriptor
}

// NewLiveActor builds a LiveInstance from a machine, an event-name dispatcher, and a descriptor function (for diagram rendering).
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

// Restore rebuilds the actor from a persisted snapshot (used by /undo and /import).
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
		Path:    activePath,
		Context: ctxBytes,
		Status:  snap.Status,
		ASCII:   sc.RenderASCII(d, sc.RenderOptions{Highlight: hl}),
	}
}

func (e *liveActor[Ctx, Evt]) Persist() ([]byte, error) {
	return e.actor.Persist()
}

func (e *liveActor[Ctx, Evt]) AvailableEvents() []string {
	d := e.describe()
	path := e.actor.Snapshot().Value.Path()
	seen := map[string]struct{}{}
	var evts []string
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

// ErrUnknownEvent is the canonical error a dispatch func returns for an unrecognised event name.
type ErrUnknownEvent struct{ Name string }

func (e ErrUnknownEvent) Error() string { return fmt.Sprintf("unknown event %q", e.Name) }

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
