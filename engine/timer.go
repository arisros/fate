package engine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/arisros/fate/effect"
	"github.com/arisros/fate/persist"
)

func makeTimerID(path []string, idx int, delayNanos int64) effect.TimerID {
	var b strings.Builder
	b.WriteString(strings.Join(path, "."))
	b.WriteString("#after#")
	b.WriteString(strconv.FormatInt(delayNanos, 10))
	b.WriteString("#")
	b.WriteString(strconv.Itoa(idx))
	return effect.TimerID(b.String())
}

// PendingTimers returns the actor's currently-armed delayed transitions, in deterministic order (by TimerID).
func (a *Actor[Ctx, Evt]) PendingTimers() []effect.PendingTimer {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]effect.PendingTimer, 0, len(a.armed))
	for id, b := range a.armed {
		out = append(out, effect.PendingTimer{ID: id, Delay: b.entry.delay, State: dotPath(b.node)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// FireTimer fires the armed delayed transition with the given id.
func (a *Actor[Ctx, Evt]) FireTimer(id effect.TimerID) bool {
	defer a.deliver()
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
	a.armed = map[effect.TimerID]afterBinding[Ctx, Evt]{}
}

func (a *Actor[Ctx, Evt]) fireTimerLocked(id effect.TimerID) bool {
	if a.status != persist.StatusRunning {
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
	a.beginStepLocked(StepTimer, "", string(id))
	a.handleAfterLocked(binding.node, binding.entry, zeroEvt)
	a.endStepLocked()
	return true
}

func (a *Actor[Ctx, Evt]) handleAfterLocked(source *stateNode[Ctx, Evt], ae afterEntry[Ctx, Evt], evt Evt) {
	for _, t := range ae.transitions {
		if !transitionPasses(t, a.ctx, evt, a.value) {
			continue
		}
		if t.Target == "" {
			a.recordTransitionLocked(source, nil, true, nil, nil)
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
