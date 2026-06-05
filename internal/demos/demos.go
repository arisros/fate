// Package demos provides a small set of generic statechart machines used by the
// scs and scs-web binaries to showcase the engine and studio. They are
// illustrative shapes — a traffic light, a media player, a build pipeline, and a
// document editor — chosen to exercise compound, parallel, final, and
// deep-history features.
package demos

import (
	"github.com/arisros/fate"
	"github.com/arisros/fate/studio"
)

// Ctx is the (empty) context shared by the demo machines.
type Ctx struct{}

// Evt is the demo event interface. Each event reports a stable EventName so the
// descriptor and studio show readable labels.
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

// Dispatch maps an event name from the studio UI to a typed demo event.
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

func must(m *fate.Machine[Ctx, Evt], err error) *fate.Machine[Ctx, Evt] {
	if err != nil {
		panic(err)
	}
	return m
}

// Demo pairs a machine with display metadata for registration.
type Demo struct {
	Name    string
	Summary string
	Machine func() *fate.Machine[Ctx, Evt]
}

// All returns every demo machine, in a stable order.
func All() []Demo {
	return []Demo{
		{"traffic-light", "Compound cycle: red → green → yellow → red.", TrafficLight},
		{"media-player", "Three parallel regions (audio, video, captions) active at once.", MediaPlayer},
		{"pipeline", "Linear build pipeline: ingest → validate → transform → done.", Pipeline},
		{"editor", "Deep history: suspend editing, then resume the exact sub-state.", Editor},
	}
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

// LiveEntry builds a studio.Entry (static + live) for a demo.
func LiveEntry(d Demo) studio.Entry {
	return studio.Entry{
		Name:    d.Name,
		Summary: d.Summary,
		Build:   d.Machine().Describe,
		BuildLive: func() studio.LiveInstance {
			return studio.NewLiveActor(d.Machine(), Dispatch, d.Machine().Describe)
		},
	}
}
