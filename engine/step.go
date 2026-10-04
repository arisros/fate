package engine

import (
	"strings"

	"github.com/arisros/fate/persist"
)

// StepCause says what set a step off.
type StepCause string

// The causes a Step can have.
const (
	// StepStart is Actor.Start entering the initial configuration.
	StepStart StepCause = "start"
	// StepEvent is an event passed to Actor.Send.
	StepEvent StepCause = "event"
	// StepRaise is an event an action raised, taken from the internal queue.
	StepRaise StepCause = "raise"
	// StepDone is an OnDone transition fired by a completed state.
	StepDone StepCause = "done"
	// StepTimer is a delayed transition delivered by Actor.FireTimer.
	StepTimer StepCause = "timer"
	// StepInvoke is the event an invocation outcome mapped to.
	StepInvoke StepCause = "invoke"
)

// StepTransition is one transition that fired in a step.
type StepTransition struct {
	Source   string `json:"source"`
	Target   string `json:"target,omitempty"`
	Internal bool   `json:"internal,omitempty"`
}

// Step records what one step of an actor did.
type Step struct {
	Seq         uint64             `json:"seq"`
	Cause       StepCause          `json:"cause"`
	Event       string             `json:"event,omitempty"`
	Effect      string             `json:"effect,omitempty"`
	Transitions []StepTransition   `json:"transitions,omitempty"`
	Exited      []string           `json:"exited,omitempty"`
	Entered     []string           `json:"entered,omitempty"`
	Value       persist.StateValue `json:"value"`
}

// SubscribeSteps registers an observer that receives every Step, in order, before the snapshot observers of [Actor.Subscribe] run.
// Runs locked: must not call the actor or block.
func (a *Actor[Ctx, Evt]) SubscribeSteps(obs func(Step)) func() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stepSubscribers = append(a.stepSubscribers, obs)
	idx := len(a.stepSubscribers) - 1
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.stepSubscribers[idx] = nil
	}
}

func (a *Actor[Ctx, Evt]) beginStepLocked(cause StepCause, event, effect string) {
	a.step = &Step{Cause: cause, Event: event, Effect: effect}
}

func (a *Actor[Ctx, Evt]) endStepLocked() {
	s := a.step
	a.step = nil
	if s == nil || (len(s.Transitions) == 0 && len(s.Entered) == 0) {
		return
	}
	a.seq++
	s.Seq = a.seq
	s.Value = a.value
	if len(a.stepSubscribers) > 0 {
		a.steps = append(a.steps, *s)
	}
}

func (a *Actor[Ctx, Evt]) recordTransitionLocked(source, target *stateNode[Ctx, Evt], internal bool, exit, entry []*stateNode[Ctx, Evt]) {
	if a.step == nil {
		return
	}
	t := StepTransition{Source: dotPath(source), Internal: internal}
	if target != nil {
		t.Target = dotPath(target)
	}
	a.step.Transitions = append(a.step.Transitions, t)
	for _, n := range exit {
		a.step.Exited = append(a.step.Exited, dotPath(n))
	}
	for _, n := range entry {
		a.step.Entered = append(a.step.Entered, dotPath(n))
	}
}

func (a *Actor[Ctx, Evt]) deliverStepsLocked() {
	steps := a.steps
	a.steps = nil
	for _, s := range steps {
		for _, obs := range a.stepSubscribers {
			if obs != nil {
				obs(s)
			}
		}
	}
}

func dotPath[Ctx any, Evt any](n *stateNode[Ctx, Evt]) string {
	return strings.Join(n.path, ".")
}
