package fate

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// TimerID uniquely identifies one armed delayed ("after") transition for as long as it is pending.
type TimerID string

// PendingTimer describes one delayed ("after") transition the actor currently has armed.
type PendingTimer struct {
	ID    TimerID
	Delay time.Duration
}

func makeTimerID(path []string, idx int, delayNanos int64) TimerID {
	var b strings.Builder
	b.WriteString(strings.Join(path, "."))
	b.WriteString("#after#")
	b.WriteString(strconv.FormatInt(delayNanos, 10))
	b.WriteString("#")
	b.WriteString(strconv.Itoa(idx))
	return TimerID(b.String())
}

// PendingTimers returns the actor's currently-armed delayed transitions, in deterministic order (by TimerID).
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

// FireTimer fires the armed delayed transition with the given id.
func (a *Actor[Ctx, Evt]) FireTimer(id TimerID) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	fired := a.fireTimerLocked(id)
	a.drainQueueLocked()
	var zeroEvt Evt
	a.settleFinalLocked(zeroEvt)
	a.notifyLocked()
	return fired
}

func (a *Actor[Ctx, Evt]) armAfterLocked(n *stateNode[Ctx, Evt]) {
	for i, ae := range n.after {
		id := makeTimerID(n.path, i, int64(ae.delay))
		a.armed[id] = afterBinding[Ctx, Evt]{node: n, entry: ae}
	}
}

func (a *Actor[Ctx, Evt]) cancelAfterLocked(n *stateNode[Ctx, Evt]) {
	for i, ae := range n.after {
		id := makeTimerID(n.path, i, int64(ae.delay))
		delete(a.armed, id)
	}
}

func (a *Actor[Ctx, Evt]) cancelAllAfterLocked() {
	a.armed = map[TimerID]afterBinding[Ctx, Evt]{}
}

func (a *Actor[Ctx, Evt]) fireTimerLocked(id TimerID) bool {
	if a.status != StatusRunning {
		return false
	}
	binding, ok := a.armed[id]
	if !ok {
		return false
	}
	delete(a.armed, id)
	// A late timer callback can race an exit.
	if _, active := extractValueAt[Ctx, Evt](a.machine.root, a.value, binding.node); !active {
		return false
	}

	var zeroEvt Evt
	a.handleAfterLocked(binding.node, binding.entry, zeroEvt)
	return true
}

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
