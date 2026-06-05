package studio_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fate "github.com/arisros/fate"
	"github.com/arisros/fate/studio"
)

type sCtx struct{}
type sEvt interface{ isS() }
type sNext struct{}

func (sNext) isS()              {}
func (sNext) EventName() string { return "NEXT" }

func demoServer(t *testing.T) *studio.Server {
	t.Helper()
	build := func() *fate.Machine[sCtx, sEvt] {
		m, err := fate.CreateMachine(fate.MachineConfig[sCtx, sEvt]{
			ID:      "demo",
			Initial: "a",
			States: map[string]fate.StateNodeConfig[sCtx, sEvt]{
				"a": {On: map[string][]fate.TransitionConfig[sCtx, sEvt]{"NEXT": {{Target: "b"}}}},
				"b": {On: map[string][]fate.TransitionConfig[sCtx, sEvt]{"NEXT": {{Target: "c"}}}},
				"c": {Type: fate.NodeFinal},
			},
		})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return m
	}
	dispatch := func(name string) (sEvt, error) {
		if name == "NEXT" {
			return sNext{}, nil
		}
		return nil, studio.ErrUnknownEvent{Name: name}
	}
	srv := studio.NewServer("test-studio")
	srv.Register(studio.Entry{
		Name:    "demo",
		Summary: "a → b → c",
		Build:   build().Describe,
		BuildLive: func() studio.LiveInstance {
			return studio.NewLiveActor(build(), dispatch, build().Describe)
		},
	})
	return srv
}

func TestStaticEndpoints(t *testing.T) {
	h := demoServer(t).Handler()
	cases := []struct {
		path string
		code int
		body string // optional substring
	}{
		{"/", 200, "demo"},
		{"/m/demo", 200, "State diagram"},
		{"/m/demo/describe", 200, `"demo"`},
		{"/m/demo/graph", 200, `"nodes"`},
		{"/m/demo/state/a", 200, "Transitions from"},
		{"/m/nope", 404, ""},
		{"/healthz", 200, "ok"},
		{"/assets/app.css", 200, ""},
		{"/assets/nope.xyz", 404, ""},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest("GET", c.path, nil))
			if rr.Code != c.code {
				t.Fatalf("%s: code %d, want %d", c.path, rr.Code, c.code)
			}
			if c.body != "" && !strings.Contains(rr.Body.String(), c.body) {
				t.Fatalf("%s: body missing %q", c.path, c.body)
			}
		})
	}
}

func TestSimulatorSessionFlow(t *testing.T) {
	ts := httptest.NewServer(demoServer(t).Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Open the simulator page (mints the session cookie).
	doGet(t, client, ts.URL+"/sim/demo", 200)

	// Send NEXT twice: a → b → c.
	doPost(t, client, ts.URL+"/sim/demo/send", "event=NEXT", 200, `"b"`)
	doPost(t, client, ts.URL+"/sim/demo/send", "event=NEXT", 200, `"c"`)

	// Undo returns to b.
	doPost(t, client, ts.URL+"/sim/demo/send", "", 0, "") // warm cookie (no-op send is 400; ignore)
	doPost(t, client, ts.URL+"/sim/demo/undo", "", 200, `"b"`)

	// Timeline lists the recorded snapshots.
	doGet(t, client, ts.URL+"/sim/demo/timeline", 200)

	// Export the snapshot, then import it back.
	exp := doGet(t, client, ts.URL+"/sim/demo/export", 200)
	doPost(t, client, ts.URL+"/sim/demo/import", exp, 200, "")

	// Reset returns to the initial state a.
	doPost(t, client, ts.URL+"/sim/demo/reset", "", 200, `"a"`)

	// Unknown event is a 400.
	doPost(t, client, ts.URL+"/sim/demo/send", "event=BOGUS", 400, "")
	// GET on a POST-only endpoint is 405.
	doGet(t, client, ts.URL+"/sim/demo/send", 405)
}

func TestSimulatorSSEStream(t *testing.T) {
	ts := httptest.NewServer(demoServer(t).Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	doGet(t, client, ts.URL+"/sim/demo", 200) // mint cookie

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/sim/demo/stream", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("stream content-type = %q", ct)
	}
	// The handler writes the current snapshot immediately as the first SSE event.
	sc := bufio.NewScanner(resp.Body)
	var got string
	for sc.Scan() {
		if line := sc.Text(); strings.HasPrefix(line, "data: ") {
			got = line
			break
		}
	}
	cancel()
	if !strings.Contains(got, `"a"`) {
		t.Fatalf("first SSE event should carry the initial state, got %q", got)
	}
}

// effCtx/effEvt drive a machine with both an after-timer and an invocation, so
// the /timer and /invoke endpoints can be exercised.
type effCtx struct{}
type effEvt interface{ isEff() }
type effDone struct{}
type effFail struct{}

func (effDone) isEff()            {}
func (effFail) isEff()            {}
func (effDone) EventName() string { return "DONE" }
func (effFail) EventName() string { return "FAIL" }

func effServer(t *testing.T) *studio.Server {
	t.Helper()
	build := func() *fate.Machine[effCtx, effEvt] {
		m, err := fate.CreateMachine(fate.MachineConfig[effCtx, effEvt]{
			ID:      "eff",
			Initial: "loading",
			States: map[string]fate.StateNodeConfig[effCtx, effEvt]{
				"loading": {
					Invoke: []fate.Invocation[effCtx, effEvt]{{
						ID: "req", Src: "svc",
						OnDone:  func(any) effEvt { return effDone{} },
						OnError: func(error) effEvt { return effFail{} },
					}},
					After: map[time.Duration][]fate.TransitionConfig[effCtx, effEvt]{
						time.Minute: {{Target: "expired"}},
					},
					On: map[string][]fate.TransitionConfig[effCtx, effEvt]{
						"DONE": {{Target: "ready"}},
						"FAIL": {{Target: "failed"}},
					},
				},
				"ready":   {Type: fate.NodeFinal},
				"failed":  {Type: fate.NodeFinal},
				"expired": {Type: fate.NodeFinal},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	dispatch := func(string) (effEvt, error) { return nil, studio.ErrUnknownEvent{Name: "none"} }
	srv := studio.NewServer("eff")
	srv.Register(studio.Entry{
		Name: "eff", Summary: "timer + invoke",
		Build:     build().Describe,
		BuildLive: func() studio.LiveInstance { return studio.NewLiveActor(build(), dispatch, build().Describe) },
	})
	return srv
}

func TestSimulatorTimerAndInvoke(t *testing.T) {
	ts := httptest.NewServer(effServer(t).Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}

	// Mint session; the SSE snapshot should advertise both pending effects.
	body := doGet(t, c, ts.URL+"/sim/eff", 200)
	_ = body
	stream := doGet(t, c, ts.URL+"/sim/eff/timeline", 200)
	_ = stream

	// Resolving the invocation drives loading → ready.
	doPost(t, c, ts.URL+"/sim/eff/invoke",
		"id=loading%23invoke%23req&action=resolve&output=true", 200, `"ready"`)

	// Fresh session: firing the timer drives loading → expired.
	c2 := newClient(ts)
	doPost(t, c2, ts.URL+"/sim/eff/timer",
		"id=loading%23after%2360000000000%230", 200, `"expired"`)

	// Fresh session: rejecting the invocation drives loading → failed.
	c3 := newClient(ts)
	doPost(t, c3, ts.URL+"/sim/eff/invoke",
		"id=loading%23invoke%23req&action=reject&error=boom", 200, `"failed"`)

	// Error paths.
	c4 := newClient(ts)
	doPost(t, c4, ts.URL+"/sim/eff/timer", "", 400, "")                                       // missing id
	doPost(t, c4, ts.URL+"/sim/eff/invoke", "id=loading%23invoke%23req&output=nope", 400, "") // bad JSON output
	doGet(t, c4, ts.URL+"/sim/eff/timer", 405)                                                // GET on POST-only
}

func newClient(ts *httptest.Server) *http.Client {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	_, _ = c.Get(ts.URL + "/sim/eff") // mint session cookie
	return c
}

// ----- helpers -----

func doGet(t *testing.T, c *http.Client, url string, wantCode int) string {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantCode {
		t.Fatalf("GET %s: code %d, want %d", url, resp.StatusCode, wantCode)
	}
	return string(body)
}

func doPost(t *testing.T, c *http.Client, url, form string, wantCode int, wantSub string) {
	t.Helper()
	resp, err := c.Post(url, "application/x-www-form-urlencoded", strings.NewReader(form))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if wantCode != 0 && resp.StatusCode != wantCode {
		t.Fatalf("POST %s: code %d, want %d (body %s)", url, resp.StatusCode, wantCode, body)
	}
	if wantSub != "" && !strings.Contains(string(body), wantSub) {
		t.Fatalf("POST %s: body missing %q (got %s)", url, wantSub, body)
	}
}
