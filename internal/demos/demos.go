// Package demos provides a small set of generic statechart machines used by the
// fate and fate-studio binaries to showcase the engine and studio. They are
// illustrative shapes — a traffic light, a media player, a build pipeline, a
// deep-history document editor, and a live-context counter — chosen to exercise
// compound, parallel, final, deep-history, and context-mutation features.
package demos

import (
	"github.com/arisros/fate"
	"github.com/arisros/fate/studio"
)

// Demo is a registrable demo machine: a name, a one-line summary, and type-
// erased builders for the live studio (Entry) and the CLI (Descriptor). The
// erasure lets demos carry different context/event types behind one slice.
type Demo struct {
	Name       string
	Summary    string
	Entry      func() studio.Entry
	Descriptor func() fate.MachineDescriptor
}

// demoFor wraps a typed machine builder and event dispatcher as a Demo,
// erasing the concrete Ctx/Evt type parameters.
func demoFor[C any, E any](
	name, summary string,
	build func() *fate.Machine[C, E],
	dispatch func(string) (E, error),
) Demo {
	return Demo{
		Name:    name,
		Summary: summary,
		Entry: func() studio.Entry {
			return studio.Entry{
				Name:    name,
				Summary: summary,
				Build:   build().Describe,
				BuildLive: func() studio.LiveInstance {
					return studio.NewLiveActor(build(), dispatch, build().Describe)
				},
			}
		},
		Descriptor: func() fate.MachineDescriptor { return build().Describe() },
	}
}

// All returns every demo, in a stable order.
func All() []Demo {
	return []Demo{
		demoFor("traffic-light", "Compound cycle: red → green → yellow → red.", TrafficLight, Dispatch),
		demoFor("media-player", "Three parallel regions (audio, video, captions) active at once.", MediaPlayer, Dispatch),
		demoFor("pipeline", "Linear build pipeline: ingest → validate → transform → done.", Pipeline, Dispatch),
		demoFor("editor", "Deep history: suspend editing, then resume the exact sub-state.", Editor, Dispatch),
		demoFor("counter", "Live context: increment, decrement, and reset a counter.", Counter, CounterDispatch),
	}
}

func must[C any, E any](m *fate.Machine[C, E], err error) *fate.Machine[C, E] {
	if err != nil {
		panic(err)
	}
	return m
}

// ----- structural demos (shared empty context, NEXT/SUSPEND/RESUME events) -----

// Ctx is the (empty) context shared by the structural demo machines.
type Ctx struct{}

// Evt is the structural-demo event interface. Each event reports a stable
// EventName so the descriptor and studio show readable labels.
type Evt interface{ isEvt() }

type evtNext struct{}
type evtSuspend struct{}
type evtResume struct{}

func (evtNext) isEvt()    {}
func (evtSuspend) isEvt() {}
func (evtResume) isEvt()  {}

func (evtNext) EventName() string    { return "NEXT" }
func (evtSuspend) EventName() string { return "SUSPEND" }
func (evtResume) EventName() string  { return "RESUME" }

// Dispatch maps an event name from the studio UI to a typed structural event.
func Dispatch(name string) (Evt, error) {
	switch name {
	case "NEXT":
		return evtNext{}, nil
	case "SUSPEND":
		return evtSuspend{}, nil
	case "RESUME":
		return evtResume{}, nil
	}
	return nil, studio.ErrUnknownEvent{Name: name}
}

// TrafficLight is a flat three-state cycle driven by NEXT.
func TrafficLight() *fate.Machine[Ctx, Evt] {
	link := func(target string) fate.StateNodeConfig[Ctx, Evt] {
		return fate.StateNodeConfig[Ctx, Evt]{On: map[string][]fate.TransitionConfig[Ctx, Evt]{
			"NEXT": {{Target: target}},
		}}
	}
	return must(fate.CreateMachine(fate.MachineConfig[Ctx, Evt]{
		ID:      "traffic-light",
		Initial: "red",
		States: map[string]fate.StateNodeConfig[Ctx, Evt]{
			"red":    link("green"),
			"green":  link("yellow"),
			"yellow": link("red"),
		},
	}))
}

// MediaPlayer is three independent parallel regions, each a small work → done
// compound, all active simultaneously.
func MediaPlayer() *fate.Machine[Ctx, Evt] {
	region := func(work string) fate.StateNodeConfig[Ctx, Evt] {
		return fate.StateNodeConfig[Ctx, Evt]{
			Initial: work,
			States: map[string]fate.StateNodeConfig[Ctx, Evt]{
				work:   {On: map[string][]fate.TransitionConfig[Ctx, Evt]{"NEXT": {{Target: "done"}}}},
				"done": {Type: fate.NodeFinal},
			},
		}
	}
	return must(fate.CreateMachine(fate.MachineConfig[Ctx, Evt]{
		ID:      "media-player",
		Initial: "playing",
		States: map[string]fate.StateNodeConfig[Ctx, Evt]{
			"playing": {
				Type: fate.NodeParallel,
				States: map[string]fate.StateNodeConfig[Ctx, Evt]{
					"audio":    region("decoding_audio"),
					"captions": region("rendering_captions"),
					"video":    region("decoding_video"),
				},
			},
		},
	}))
}

// Pipeline is a linear flow ending in a final state.
func Pipeline() *fate.Machine[Ctx, Evt] {
	link := func(target string) fate.StateNodeConfig[Ctx, Evt] {
		return fate.StateNodeConfig[Ctx, Evt]{On: map[string][]fate.TransitionConfig[Ctx, Evt]{
			"NEXT": {{Target: target}},
		}}
	}
	return must(fate.CreateMachine(fate.MachineConfig[Ctx, Evt]{
		ID:      "pipeline",
		Initial: "ingest",
		States: map[string]fate.StateNodeConfig[Ctx, Evt]{
			"ingest":    link("validate"),
			"validate":  link("transform"),
			"transform": link("done"),
			"done":      {Type: fate.NodeFinal},
		},
	}))
}

// Editor showcases deep history: the editing flow can be suspended at any
// sub-state and resumed exactly where it left off via a deep-history node.
func Editor() *fate.Machine[Ctx, Evt] {
	link := func(target string) fate.StateNodeConfig[Ctx, Evt] {
		return fate.StateNodeConfig[Ctx, Evt]{On: map[string][]fate.TransitionConfig[Ctx, Evt]{
			"NEXT": {{Target: target}},
		}}
	}
	return must(fate.CreateMachine(fate.MachineConfig[Ctx, Evt]{
		ID:      "editor",
		Initial: "session",
		States: map[string]fate.StateNodeConfig[Ctx, Evt]{
			"session": {
				Initial: "editing",
				States: map[string]fate.StateNodeConfig[Ctx, Evt]{
					"editing": {
						Initial: "draft",
						States: map[string]fate.StateNodeConfig[Ctx, Evt]{
							"draft":      link("review"),
							"review":     link("publishing"),
							"publishing": link("done"),
							"hist":       {Type: fate.NodeHistory, History: fate.HistoryDeep, Default: "draft"},
						},
						On: map[string][]fate.TransitionConfig[Ctx, Evt]{
							"SUSPEND": {{Target: "suspended"}},
						},
					},
					"suspended": {On: map[string][]fate.TransitionConfig[Ctx, Evt]{
						"RESUME": {{Target: "editing.hist"}},
					}},
					"done": {Type: fate.NodeFinal},
				},
			},
		},
	}))
}

// ----- counter demo (live context: INC / DEC / RESET) -----

// CounterCtx is the counter's context; its Count field is what the studio's
// context panel displays and updates live as events are sent.
type CounterCtx struct {
	Count int `json:"count"`
}

// CounterEvt is the counter's event interface.
type CounterEvt interface{ isCounterEvt() }

type cInc struct{}
type cDec struct{}
type cReset struct{}

func (cInc) isCounterEvt()   {}
func (cDec) isCounterEvt()   {}
func (cReset) isCounterEvt() {}

func (cInc) EventName() string   { return "INC" }
func (cDec) EventName() string   { return "DEC" }
func (cReset) EventName() string { return "RESET" }

// CounterDispatch maps an event name from the studio UI to a counter event.
func CounterDispatch(name string) (CounterEvt, error) {
	switch name {
	case "INC":
		return cInc{}, nil
	case "DEC":
		return cDec{}, nil
	case "RESET":
		return cReset{}, nil
	}
	return nil, studio.ErrUnknownEvent{Name: name}
}

// Counter is a single-state machine whose transitions mutate context, so the
// studio's context panel shows {"count": N} changing live as you send events.
func Counter() *fate.Machine[CounterCtx, CounterEvt] {
	add := func(d int) fate.Action[CounterCtx, CounterEvt] {
		return fate.Assign(func(c CounterCtx, _ CounterEvt) CounterCtx { c.Count += d; return c })
	}
	return must(fate.CreateMachine(fate.MachineConfig[CounterCtx, CounterEvt]{
		ID:      "counter",
		Initial: "active",
		States: map[string]fate.StateNodeConfig[CounterCtx, CounterEvt]{
			"active": {On: map[string][]fate.TransitionConfig[CounterCtx, CounterEvt]{
				"INC": {{Actions: []fate.Action[CounterCtx, CounterEvt]{add(1)}}},
				"DEC": {{Actions: []fate.Action[CounterCtx, CounterEvt]{add(-1)}}},
				"RESET": {{Actions: []fate.Action[CounterCtx, CounterEvt]{
					fate.Assign(func(c CounterCtx, _ CounterEvt) CounterCtx { c.Count = 0; return c }),
				}}},
			}},
		},
	}))
}
