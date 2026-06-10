package render_test

// Golden-file tests for ASCII. Each test case builds a small but representative
// machine, renders it, and compares to a checked-in .golden file under
// testdata/ascii_graph/. To regenerate the golden files after a renderer
// change, run:
//
//	go test -count=1 -run TestASCII_Golden ./render/... -update
//
// The -update flag is parsed by this file's init() — it's a *testing* flag,
// not a build flag, so it doesn't affect normal runs.

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	sc "github.com/arisros/fate"
	"github.com/arisros/fate/render"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files under testdata/ascii_graph/")

type goldenCase struct {
	name       string
	build      func(t *testing.T) sc.MachineDescriptor
	highlight  map[string]rune
	goldenFile string
}

func goldenCases() []goldenCase {
	return []goldenCase{
		{
			name:       "traffic_light",
			build:      buildTrafficLightFixture,
			goldenFile: "traffic_light.golden",
		},
		{
			name:       "traffic_light_highlighted_yellow",
			build:      buildTrafficLightFixture,
			highlight:  map[string]rune{"yellow": '▶'},
			goldenFile: "traffic_light_highlighted_yellow.golden",
		},
		{
			name:       "parallel_regions",
			build:      buildParallelGoldenFixture,
			goldenFile: "parallel_regions.golden",
		},
		{
			name:       "linear_pipeline",
			build:      buildLinearGoldenFixture,
			goldenFile: "linear_pipeline.golden",
		},
	}
}

func TestASCII_Golden(t *testing.T) {
	for _, c := range goldenCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			d := c.build(t)
			got := render.ASCII(d, render.Options{Highlight: c.highlight})
			path := filepath.Join("testdata", "ascii_graph", c.goldenFile)
			if *updateGolden {
				if err := os.WriteFile(path, []byte(got), 0644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("updated %s", path)
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with -update to create)", path, err)
			}
			if string(want) != got {
				t.Errorf("ASCII renderer output drifted from %s.\n--- want ---\n%s\n--- got ---\n%s", path, want, got)
			}
		})
	}
}

type gldCtx struct{}
type gldEvt interface{ isGldEvt() }
type gldEvtNext struct{}

func (gldEvtNext) isGldEvt()         {}
func (gldEvtNext) EventName() string { return "NEXT" }

func buildTrafficLightFixture(t *testing.T) sc.MachineDescriptor {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[gldCtx, gldEvt]{
		ID:      "traffic-light",
		Initial: "red",
		States: map[string]sc.StateNodeConfig[gldCtx, gldEvt]{
			"red":    {On: map[string][]sc.TransitionConfig[gldCtx, gldEvt]{"NEXT": {{Target: "green"}}}},
			"green":  {On: map[string][]sc.TransitionConfig[gldCtx, gldEvt]{"NEXT": {{Target: "yellow"}}}},
			"yellow": {On: map[string][]sc.TransitionConfig[gldCtx, gldEvt]{"NEXT": {{Target: "red"}}}},
		},
	})
	if err != nil {
		t.Fatalf("traffic-light: %v", err)
	}
	return m.Describe()
}

func buildParallelGoldenFixture(t *testing.T) sc.MachineDescriptor {
	t.Helper()
	region := func(workName string) sc.StateNodeConfig[gldCtx, gldEvt] {
		return sc.StateNodeConfig[gldCtx, gldEvt]{
			Initial: workName,
			States: map[string]sc.StateNodeConfig[gldCtx, gldEvt]{
				workName: {On: map[string][]sc.TransitionConfig[gldCtx, gldEvt]{
					"NEXT": {{Target: "done"}},
				}},
				"done": {Type: sc.NodeFinal},
			},
		}
	}
	m, err := sc.CreateMachine(sc.MachineConfig[gldCtx, gldEvt]{
		ID:      "media-player",
		Initial: "playing",
		States: map[string]sc.StateNodeConfig[gldCtx, gldEvt]{
			"playing": {
				Type: sc.NodeParallel,
				States: map[string]sc.StateNodeConfig[gldCtx, gldEvt]{
					"audio":    region("decoding_audio"),
					"captions": region("rendering_captions"),
					"video":    region("decoding_video"),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("media-player: %v", err)
	}
	return m.Describe()
}

func buildLinearGoldenFixture(t *testing.T) sc.MachineDescriptor {
	t.Helper()
	m, err := sc.CreateMachine(sc.MachineConfig[gldCtx, gldEvt]{
		ID:      "pipeline",
		Initial: "ingest",
		States: map[string]sc.StateNodeConfig[gldCtx, gldEvt]{
			"ingest": {On: map[string][]sc.TransitionConfig[gldCtx, gldEvt]{
				"NEXT": {{Target: "validate"}},
			}},
			"validate": {On: map[string][]sc.TransitionConfig[gldCtx, gldEvt]{
				"NEXT": {{Target: "transform"}},
			}},
			"transform": {On: map[string][]sc.TransitionConfig[gldCtx, gldEvt]{
				"NEXT": {{Target: "done"}},
			}},
			"done": {Type: sc.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	return m.Describe()
}
