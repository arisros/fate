package httphandler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/arisros/fate"
	"github.com/arisros/fate/httphandler"
)

// ----- test machine -----
//
// idle --GO--> running --STOP--> idle
// idle --after 10ms--> timedOut
// running invokes "work": OnDone -> done, OnError -> failed

type demoCtx struct {
	Count int `json:"count"`
}

type demoEvt interface{ isDemoEvt() }

type evtGo struct{}
type evtStop struct{}
type evtWorkDone struct{}
type evtWorkFailed struct{}

func (evtGo) isDemoEvt()                {}
func (evtGo) EventName() string         { return "GO" }
func (evtStop) isDemoEvt()              {}
func (evtStop) EventName() string       { return "STOP" }
func (evtWorkDone) isDemoEvt()          {}
func (evtWorkDone) EventName() string   { return "WORK_DONE" }
func (evtWorkFailed) isDemoEvt()        {}
func (evtWorkFailed) EventName() string { return "WORK_FAILED" }

func demoMachine(t *testing.T) *fate.Machine[demoCtx, demoEvt] {
	t.Helper()
	m, err := fate.CreateMachine(fate.MachineConfig[demoCtx, demoEvt]{
		ID:      "demo",
		Initial: "idle",
		States: map[string]fate.StateNodeConfig[demoCtx, demoEvt]{
			"idle": {
				On: map[string][]fate.TransitionConfig[demoCtx, demoEvt]{
					"GO": {{
						Target:  "running",
						Actions: []fate.Action[demoCtx, demoEvt]{fate.Assign(func(c demoCtx, _ demoEvt) demoCtx { c.Count++; return c })},
					}},
				},
				After: map[time.Duration][]fate.TransitionConfig[demoCtx, demoEvt]{
					10 * time.Millisecond: {{Target: "timedOut"}},
				},
			},
			"running": {
				Invoke: []fate.Invocation[demoCtx, demoEvt]{{
					ID:      "work",
					Src:     "activity:work",
					OnDone:  func(any) demoEvt { return evtWorkDone{} },
					OnError: func(error) demoEvt { return evtWorkFailed{} },
				}},
				On: map[string][]fate.TransitionConfig[demoCtx, demoEvt]{
					"STOP":        {{Target: "idle"}},
					"WORK_DONE":   {{Target: "done"}},
					"WORK_FAILED": {{Target: "failed"}},
				},
			},
			"timedOut": {},
			"done":     {Type: fate.NodeFinal},
			"failed":   {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	return m
}

func demoDispatch(name string) (demoEvt, error) {
	switch name {
	case "GO":
		return evtGo{}, nil
	case "STOP":
		return evtStop{}, nil
	}
	return nil, httphandler.ErrUnknownEvent{Name: name}
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(httphandler.New(demoMachine(t), demoDispatch))
	t.Cleanup(srv.Close)
	return srv
}

// client retains cookies so the session sticks across calls.
func newClient() *http.Client {
	return &http.Client{Jar: cookieJar{m: map[string][]*http.Cookie{}}}
}

// minimal in-memory cookie jar (avoids net/http/cookiejar's PSL dependency).
type cookieJar struct{ m map[string][]*http.Cookie }

func (j cookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) { j.m[u.Host] = cookies }
func (j cookieJar) Cookies(u *url.URL) []*http.Cookie             { return j.m[u.Host] }

// ----- request helpers (all check errors so errcheck/govet stay quiet) -----

func post(t *testing.T, c *http.Client, path string, v url.Values) *http.Response {
	t.Helper()
	resp, err := c.PostForm(path, v)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func get(t *testing.T, c *http.Client, path string) *http.Response {
	t.Helper()
	resp, err := c.Get(path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func postBody(t *testing.T, c *http.Client, path, contentType, body string) *http.Response {
	t.Helper()
	resp, err := c.Post(path, contentType, strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

// snap performs a request and returns the decoded JSON snapshot body, closing it.
func snap(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

func firstInvokeID(t *testing.T, m map[string]any) string {
	t.Helper()
	invs, ok := m["invocations"].([]any)
	if !ok || len(invs) == 0 {
		t.Fatalf("no pending invocations in %v", m)
	}
	return invs[0].(map[string]any)["id"].(string)
}

// ----- tests -----

func TestSend_AdvancesState(t *testing.T) {
	srv := newServer(t)
	c := newClient()

	m := snap(t, post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}}))
	if m["path"] != "running" {
		t.Fatalf("path: got %q want running", m["path"])
	}
	if m["ascii"] == "" {
		t.Fatal("ascii should not be empty")
	}
	ctxBytes, _ := json.Marshal(m["context"])
	if !strings.Contains(string(ctxBytes), `"count":1`) {
		t.Fatalf("context should show count=1, got %s", ctxBytes)
	}
	invs := m["invocations"].([]any)
	if len(invs) != 1 || invs[0].(map[string]any)["src"] != "activity:work" {
		t.Fatalf("expected 1 pending invocation, got %+v", invs)
	}
	if len(m["events"].([]any)) == 0 {
		t.Fatal("expected available events at running")
	}
}

func TestSend_MissingEvent(t *testing.T) {
	srv := newServer(t)
	resp := post(t, newClient(), srv.URL+"/send", url.Values{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
}

func TestSend_UnknownEvent(t *testing.T) {
	srv := newServer(t)
	resp := post(t, newClient(), srv.URL+"/send", url.Values{"event": {"NOPE"}})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
}

func TestSend_WrongMethod(t *testing.T) {
	srv := newServer(t)
	resp := get(t, newClient(), srv.URL+"/send")
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d want 405", resp.StatusCode)
	}
}

func TestReset(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}}).Body.Close()
	m := snap(t, postBody(t, c, srv.URL+"/reset", "", ""))
	if m["path"] != "idle" {
		t.Fatalf("after reset path: got %v want idle", m["path"])
	}
}

func TestUndo(t *testing.T) {
	srv := newServer(t)
	c := newClient()

	// undo with empty history → 400.
	resp := postBody(t, c, srv.URL+"/undo", "", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("undo empty: got %d want 400", resp.StatusCode)
	}

	// advance, then undo → back to idle.
	post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}}).Body.Close()
	m := snap(t, postBody(t, c, srv.URL+"/undo", "", ""))
	if m["path"] != "idle" {
		t.Fatalf("after undo path: got %v want idle", m["path"])
	}
}

func TestTimeline(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}}).Body.Close()
	post(t, c, srv.URL+"/send", url.Values{"event": {"STOP"}}).Body.Close()

	resp := get(t, c, srv.URL+"/timeline")
	defer resp.Body.Close()
	var tl struct {
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tl); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(tl.Events) != 2 || tl.Events[0] != "GO" || tl.Events[1] != "STOP" {
		t.Fatalf("timeline: got %v want [GO STOP]", tl.Events)
	}
}

func TestExportImport(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}}).Body.Close()

	// export the running-state snapshot.
	expResp := get(t, c, srv.URL+"/export")
	snapBytes, err := io.ReadAll(expResp.Body)
	expResp.Body.Close()
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if len(snapBytes) == 0 {
		t.Fatal("export returned empty body")
	}

	// reset to idle, then import the snapshot back → running.
	postBody(t, c, srv.URL+"/reset", "", "").Body.Close()
	m := snap(t, postBody(t, c, srv.URL+"/import", "application/json", string(snapBytes)))
	if m["path"] != "running" {
		t.Fatalf("after import path: got %v want running", m["path"])
	}
}

func TestImport_BadSnapshot(t *testing.T) {
	srv := newServer(t)
	resp := postBody(t, newClient(), srv.URL+"/import", "application/json", "{not valid")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("import bad: got %d want 400", resp.StatusCode)
	}
}

func TestTimer(t *testing.T) {
	srv := newServer(t)
	c := newClient()

	// reset returns the snapshot including the armed timer on idle.
	m := snap(t, postBody(t, c, srv.URL+"/reset", "", ""))
	timers, _ := m["timers"].([]any)
	if len(timers) == 0 {
		t.Fatal("expected an armed timer at idle")
	}
	id := timers[0].(map[string]any)["id"].(string)

	fired := snap(t, post(t, c, srv.URL+"/timer", url.Values{"id": {id}}))
	if fired["path"] != "timedOut" {
		t.Fatalf("after timer path: got %v want timedOut", fired["path"])
	}
}

func TestTimer_MissingID(t *testing.T) {
	srv := newServer(t)
	resp := post(t, newClient(), srv.URL+"/timer", url.Values{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("timer missing id: got %d want 400", resp.StatusCode)
	}
}

func TestInvoke_Resolve(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	id := firstInvokeID(t, snap(t, post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}})))

	m := snap(t, post(t, c, srv.URL+"/invoke", url.Values{
		"id": {id}, "action": {"resolve"}, "output": {"true"},
	}))
	if m["path"] != "done" {
		t.Fatalf("after resolve path: got %v want done", m["path"])
	}
}

func TestInvoke_Reject(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	id := firstInvokeID(t, snap(t, post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}})))

	m := snap(t, post(t, c, srv.URL+"/invoke", url.Values{
		"id": {id}, "action": {"reject"}, "error": {"boom"},
	}))
	if m["path"] != "failed" {
		t.Fatalf("after reject path: got %v want failed", m["path"])
	}
}

func TestInvoke_EmptyOutputAndError(t *testing.T) {
	srv := newServer(t)

	// resolve with no output payload → out stays nil (empty-trim branch).
	c := newClient()
	id := firstInvokeID(t, snap(t, post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}})))
	m := snap(t, post(t, c, srv.URL+"/invoke", url.Values{"id": {id}, "action": {"resolve"}}))
	if m["path"] != "done" {
		t.Fatalf("resolve empty output path: got %v want done", m["path"])
	}

	// reject with no error message → default "rejected" message branch.
	c2 := newClient()
	id2 := firstInvokeID(t, snap(t, post(t, c2, srv.URL+"/send", url.Values{"event": {"GO"}})))
	m2 := snap(t, post(t, c2, srv.URL+"/invoke", url.Values{"id": {id2}, "action": {"reject"}}))
	if m2["path"] != "failed" {
		t.Fatalf("reject empty err path: got %v want failed", m2["path"])
	}
}

func TestInvoke_BadOutputJSON(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	id := firstInvokeID(t, snap(t, post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}})))

	resp := post(t, c, srv.URL+"/invoke", url.Values{
		"id": {id}, "action": {"resolve"}, "output": {"{not json"},
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invoke bad output: got %d want 400", resp.StatusCode)
	}
}

func TestInvoke_MissingID(t *testing.T) {
	srv := newServer(t)
	resp := post(t, newClient(), srv.URL+"/invoke", url.Values{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invoke missing id: got %d want 400", resp.StatusCode)
	}
}

func TestWrongMethod(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	for _, ep := range []string{"/timer", "/invoke", "/reset", "/undo", "/import"} {
		resp := get(t, c, srv.URL+ep)
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: got %d want 405", ep, resp.StatusCode)
		}
	}
}

func TestCompound_NestedAvailableEvents(t *testing.T) {
	m, err := fate.CreateMachine(fate.MachineConfig[demoCtx, demoEvt]{
		ID:      "compound",
		Initial: "outer",
		States: map[string]fate.StateNodeConfig[demoCtx, demoEvt]{
			"outer": {
				Initial: "inner",
				States: map[string]fate.StateNodeConfig[demoCtx, demoEvt]{
					"inner": {
						On: map[string][]fate.TransitionConfig[demoCtx, demoEvt]{
							"GO": {{Target: "done"}},
						},
					},
				},
			},
			"done": {Type: fate.NodeFinal},
		},
	})
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	srv := httptest.NewServer(httphandler.New(m, demoDispatch))
	defer srv.Close()
	c := newClient()

	body := snap(t, postBody(t, c, srv.URL+"/reset", "", ""))
	if body["path"] != "outer.inner" {
		t.Fatalf("nested path: got %v want outer.inner", body["path"])
	}
	found := false
	for _, e := range body["events"].([]any) {
		if e == "GO" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected GO available at outer.inner, got %v", body["events"])
	}
}

func TestStream_DeliversInitialAndUpdate(t *testing.T) {
	srv := newServer(t)
	c := newClient()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/stream", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("content-type"); ct != "text/event-stream" {
		t.Fatalf("content-type: got %q want text/event-stream", ct)
	}

	r := bufio.NewReader(resp.Body)
	if got := decodePath(t, readSSEData(t, r)); got != "idle" {
		t.Fatalf("initial stream path: got %v want idle", got)
	}

	// trigger an update on the same session; the broadcast should arrive.
	post(t, c, srv.URL+"/send", url.Values{"event": {"GO"}}).Body.Close()
	if got := decodePath(t, readSSEData(t, r)); got != "running" {
		t.Fatalf("update stream path: got %v want running", got)
	}
}

func TestStream_ClientDisconnect(t *testing.T) {
	srv := newServer(t)
	c := newClient()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/stream", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(resp.Body)
	readSSEData(t, r) // drain the initial frame
	cancel()          // client goes away → handler must return via ctx.Done
	resp.Body.Close()
}

func TestSessionIsolation(t *testing.T) {
	srv := newServer(t)
	a := newClient()
	b := newClient()

	// a advances to running.
	post(t, a, srv.URL+"/send", url.Values{"event": {"GO"}}).Body.Close()
	// b is a separate session — its own actor, still idle.
	m := snap(t, postBody(t, b, srv.URL+"/reset", "", ""))
	if m["path"] != "idle" {
		t.Fatalf("session b path: got %v want idle (sessions not isolated)", m["path"])
	}
}

func TestUnknownRoute(t *testing.T) {
	srv := newServer(t)
	resp := get(t, newClient(), srv.URL+"/bogus")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown route: got %d want 404", resp.StatusCode)
	}
}

// ----- SSE helpers -----

func decodePath(t *testing.T, data string) any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		t.Fatalf("decode SSE frame %q: %v", data, err)
	}
	return m["path"]
}

// readSSEData reads lines until a "data: ..." line and returns the payload.
func readSSEData(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	deadline := time.After(2 * time.Second)
	lines := make(chan string, 1)
	go func() {
		for {
			line, err := r.ReadString('\n')
			if strings.HasPrefix(line, "data: ") {
				lines <- strings.TrimSpace(strings.TrimPrefix(line, "data: "))
				return
			}
			if err != nil {
				lines <- ""
				return
			}
		}
	}()
	select {
	case l := <-lines:
		if l == "" {
			t.Fatal("stream closed before a data frame")
		}
		return l
	case <-deadline:
		t.Fatal("timed out waiting for SSE data frame")
		return ""
	}
}
