package studio_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	sc "github.com/arisros/fate"
)

// clientFor returns an http.Client with a cookie jar (so fate_sid persists
// across requests, like a real browser) pointed at a fresh test server.
func clientFor(t *testing.T) (*http.Client, string, func()) {
	t.Helper()
	srv := httptest.NewServer(newTestServer().Handler())
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}, srv.URL, srv.Close
}

func post(t *testing.T, c *http.Client, urlStr string, form url.Values) (int, string) {
	t.Helper()
	var body io.Reader
	ct := ""
	if form != nil {
		body = strings.NewReader(form.Encode())
		ct = "application/x-www-form-urlencoded"
	}
	req, _ := http.NewRequest(http.MethodPost, urlStr, body)
	if ct != "" {
		req.Header.Set("content-type", ct)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", urlStr, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func pathOf(t *testing.T, jsonBody string) string {
	t.Helper()
	i := strings.Index(jsonBody, `"path":"`)
	if i < 0 {
		t.Fatalf("no path in %s", jsonBody)
	}
	rest := jsonBody[i+8:]
	return rest[:strings.IndexByte(rest, '"')]
}

func TestSim_CookieMintedAndPersisted(t *testing.T) {
	c, base, closeFn := clientFor(t)
	defer closeFn()
	// GET the sim page → should Set-Cookie fate_sid.
	resp, _ := c.Get(base + "/sim/traffic-light")
	resp.Body.Close()
	u, _ := url.Parse(base)
	var found bool
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == "fate_sid" && ck.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("fate_sid cookie not set on /sim page load")
	}
}

func TestSim_PerUserIsolation(t *testing.T) {
	// Two clients (two cookie jars) must drive independent actors.
	srv := httptest.NewServer(newTestServer().Handler())
	defer srv.Close()
	jar1, _ := cookiejar.New(nil)
	jar2, _ := cookiejar.New(nil)
	c1 := &http.Client{Jar: jar1}
	c2 := &http.Client{Jar: jar2}

	// Prime cookies via the page.
	r1, _ := c1.Get(srv.URL + "/sim/traffic-light")
	r1.Body.Close()
	r2, _ := c2.Get(srv.URL + "/sim/traffic-light")
	r2.Body.Close()

	// c1 advances red→green; c2 stays at red.
	_, b1 := post(t, c1, srv.URL+"/sim/traffic-light/send", url.Values{"event": {"NEXT"}})
	if got := pathOf(t, b1); got != "green" {
		t.Fatalf("c1 after NEXT: %q", got)
	}
	// c2's stream/export should still be red.
	resp, _ := c2.Get(srv.URL + "/sim/traffic-light/export")
	eb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(eb), `"red"`) {
		t.Errorf("c2 leaked c1's state; export=%s", string(eb))
	}
}

func TestSim_UndoRestoresPriorState(t *testing.T) {
	c, base, closeFn := clientFor(t)
	defer closeFn()
	r, _ := c.Get(base + "/sim/traffic-light")
	r.Body.Close()

	_, b1 := post(t, c, base+"/sim/traffic-light/send", url.Values{"event": {"NEXT"}})
	if pathOf(t, b1) != "green" {
		t.Fatalf("expected green, got %s", b1)
	}
	code, b2 := post(t, c, base+"/sim/traffic-light/undo", nil)
	if code != 200 {
		t.Fatalf("undo code=%d body=%s", code, b2)
	}
	if got := pathOf(t, b2); got != "red" {
		t.Errorf("after undo: got %q want red", got)
	}
	// Undo with empty history → 400.
	code, _ = post(t, c, base+"/sim/traffic-light/undo", nil)
	if code != http.StatusBadRequest {
		t.Errorf("undo on empty history: code=%d want 400", code)
	}
}

func TestSim_ExportImportRoundtrip(t *testing.T) {
	c, base, closeFn := clientFor(t)
	defer closeFn()
	r, _ := c.Get(base + "/sim/traffic-light")
	r.Body.Close()

	// Advance to yellow.
	post(t, c, base+"/sim/traffic-light/send", url.Values{"event": {"NEXT"}})
	post(t, c, base+"/sim/traffic-light/send", url.Values{"event": {"NEXT"}})

	exp, _ := c.Get(base + "/sim/traffic-light/export")
	snapBytes, _ := io.ReadAll(exp.Body)
	exp.Body.Close()

	// Reset to red, then import the yellow snapshot.
	post(t, c, base+"/sim/traffic-light/reset", nil)
	req, _ := http.NewRequest(http.MethodPost, base+"/sim/traffic-light/import", strings.NewReader(string(snapBytes)))
	req.Header.Set("content-type", "application/json")
	resp, _ := c.Do(req)
	ib, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("import code=%d body=%s", resp.StatusCode, string(ib))
	}
	if got := pathOf(t, string(ib)); got != "yellow" {
		t.Errorf("after import: got %q want yellow", got)
	}
}

func TestSim_TimelineTracksEvents(t *testing.T) {
	c, base, closeFn := clientFor(t)
	defer closeFn()
	r, _ := c.Get(base + "/sim/traffic-light")
	r.Body.Close()
	post(t, c, base+"/sim/traffic-light/send", url.Values{"event": {"NEXT"}})
	post(t, c, base+"/sim/traffic-light/send", url.Values{"event": {"NEXT"}})

	resp, _ := c.Get(base + "/sim/traffic-light/timeline")
	tb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Count(string(tb), "NEXT") != 2 {
		t.Errorf("timeline should list 2 NEXT events; got %s", string(tb))
	}
}

// The resolved graph endpoint backs the self-hosted canvas.
func TestServer_GraphEndpoint(t *testing.T) {
	srv := httptest.NewServer(newTestServer().Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/m/traffic-light/graph")
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var g sc.Graph
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatalf("graph unmarshal: %v", err)
	}
	if g.ID != "traffic-light" || len(g.Nodes) != 3 || len(g.Edges) != 3 {
		t.Errorf("graph shape: id=%q nodes=%d edges=%d", g.ID, len(g.Nodes), len(g.Edges))
	}
	// initial node + a resolved edge target.
	if g.Initial == "" {
		t.Error("graph missing initial")
	}
	var hasEdge bool
	for _, e := range g.Edges {
		if e.Event == "NEXT" && e.Source != "" && e.Target != "" {
			hasEdge = true
		}
	}
	if !hasEdge {
		t.Error("graph missing a resolved NEXT edge")
	}
}

// Asset serving — elkjs (the layout engine) replaced mermaid.
func TestServer_ServesEmbeddedAssets(t *testing.T) {
	srv := httptest.NewServer(newTestServer().Handler())
	defer srv.Close()
	for _, a := range []string{"/assets/app.css", "/assets/app.js", "/assets/elk.bundled.js"} {
		resp, err := http.Get(srv.URL + a)
		if err != nil {
			t.Fatalf("get %s: %v", a, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: code=%d", a, resp.StatusCode)
		}
	}
}

// Ensure descriptor still round-trips (regression guard for the shared helper).
func TestServer_DescribeStillWorks(t *testing.T) {
	srv := httptest.NewServer(newTestServer().Handler())
	defer srv.Close()
	resp, _ := http.Get(srv.URL + "/m/traffic-light/describe")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var d sc.MachineDescriptor
	if err := json.Unmarshal(b, &d); err != nil || d.ID != "traffic-light" {
		t.Errorf("describe broke: err=%v id=%q", err, d.ID)
	}
}
