package fate

import (
	"sort"
	"strconv"
	"strings"
)

// makeTimerID derives the deterministic [TimerID] for the idx-th delay bucket
// of the state at the given path. The encoding embeds the path, the delay, and
// the index so distinct buckets never collide and the same logical timer keeps
// the same ID across runs and across persistence.
func makeTimerID(path []string, idx int, delayNanos int64) TimerID {
	var b strings.Builder
	b.WriteString(strings.Join(path, "."))
	b.WriteString("#after#")
	b.WriteString(strconv.FormatInt(delayNanos, 10))
	b.WriteString("#")
	b.WriteString(strconv.Itoa(idx))
	return TimerID(b.String())
}

// PendingTimers returns the actor's currently-armed delayed transitions, in
// deterministic order (by TimerID). It is the read half of the timer interface:
// an adapter arms its own durable or wall-clock timers from this list and calls
// [Actor.FireTimer] when a delay elapses. The core never fires a timer itself,
// so without an adapter pending timers simply remain armed. See ADR-0003.
func (a *Actor[Ctx, Evt]) PendingTimers() []PendingTimer {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]PendingTimer, 0, len(a.armed))
	for id, b := range a.armed {
		out = append(out, PendingTimer{ID: id, Delay: b.entry.delay})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FireTimer fires the armed delayed transition with the given id. It is the
// write half of the pull-based timer interface (see [Actor.PendingTimers]) and
// is how an adapter delivers an elapsed "after" delay back to the machine.
// Firing an id that is not currently armed, or firing when the owning state is
// no longer active, is a safe no-op.
func (a *Actor[Ctx, Evt]) FireTimer(id TimerID) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fireTimerLocked(id)
	a.drainQueueLocked()
	var zeroEvt Evt
	a.settleFinalLocked(zeroEvt)
	a.notifyLocked()
}

// armAfterLocked records every delayed transition declared on n as a pending
// timer. Called when n is entered (initial Start chain or a transition's entry
// set). Each bucket gets one entry keyed by a deterministic [TimerID]. The core
// does not start any clock; an adapter discovers these via PendingTimers and
// fires them via FireTimer. The actor mutex must be held.
func (a *Actor[Ctx, Evt]) armAfterLocked(n *stateNode[Ctx, Evt]) {
	for i, ae := range n.after {
		id := makeTimerID(n.path, i, int64(ae.delay))
		a.armed[id] = afterBinding[Ctx, Evt]{node: n, entry: ae}
	}
}

// cancelAfterLocked disarms every delayed transition declared on n. Called when
// n is exited. The actor mutex must be held.
func (a *Actor[Ctx, Evt]) cancelAfterLocked(n *stateNode[Ctx, Evt]) {
	for i, ae := range n.after {
		id := makeTimerID(n.path, i, int64(ae.delay))
		delete(a.armed, id)
	}
}

// cancelAllAfterLocked disarms every pending timer. Called on Stop. The actor
// mutex must be held.
func (a *Actor[Ctx, Evt]) cancelAllAfterLocked() {
	a.armed = map[TimerID]afterBinding[Ctx, Evt]{}
}

// fireTimerLocked verifies the timer is still armed and its state still active,
// then fires the matching delayed transition as an internal step. The actor
// mutex must be held. A timer cancelled (state exited) in the meantime is a
// safe no-op.
func (a *Actor[Ctx, Evt]) fireTimerLocked(id TimerID) {
	if a.status != StatusRunning {
		return
	}
	binding, ok := a.armed[id]
	if !ok {
		return // cancelled or already fired
	}
	delete(a.armed, id)
	// Defensive: only fire if the owning state is still active. (Exit cancels
	// timers, so this should always hold, but a late wall-clock callback racing
	// an exit is harmless this way.)
	if _, active := extractValueAt[Ctx, Evt](a.machine.root, a.value, binding.node); !active {
		return
	}

	var zeroEvt Evt
	a.handleAfterLocked(binding.node, binding.entry, zeroEvt)
}

// handleAfterLocked selects and applies the first transition in a fired delay
// bucket whose Guard and Cond pass, mirroring event handling. An empty Target
// runs the transition's actions in place; otherwise the actor transitions. If
// no candidate passes, the bucket is a no-op (it re-arms only when the state is
// re-entered). The actor mutex must be held.
func (a *Actor[Ctx, Evt]) handleAfterLocked(source *stateNode[Ctx, Evt], ae afterEntry[Ctx, Evt], evt Evt) {
	for _, t := range ae.transitions {
		if !transitionPasses(t, a.ctx, evt, a.value) {
			continue
		}
		if t.Target == "" {
			a.runActions(t.Actions, evt)
			return
		}
		target := resolveTarget(source, t.Target)
		if target == nil {
			return
		}
		target = a.resolveHistoryRedirect(target)
		if target == nil {
			return
		}
		a.runTransitionLocked(source, target, t, evt)
		return
	}
}
