package studio_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sc "github.com/arisros/fate"
	"github.com/arisros/fate/studio"
)

type tlCtx struct{}
type tlEvt interface{ isTLEvt() }
type tlNext struct{}

func (tlNext) isTLEvt()          {}
func (tlNext) EventName() string { return "NEXT" }

func trafficLight() *sc.Machine[tlCtx, tlEvt] {
	m, err := sc.CreateMachine(sc.MachineConfig[tlCtx, tlEvt]{
		ID:      "traffic-light",
		Initial: "red",
		States: map[string]sc.StateNodeConfig[tlCtx, tlEvt]{
			"red":    {On: map[string][]sc.TransitionConfig[tlCtx, tlEvt]{"NEXT": {{Target: "green"}}}},
			"green":  {On: map[string][]sc.TransitionConfig[tlCtx, tlEvt]{"NEXT": {{Target: "yellow"}}}},
			"yellow": {On: map[string][]sc.TransitionConfig[tlCtx, tlEvt]{"NEXT": {{Target: "red"}}}},
		},
	})
	if err != nil {
		panic(err)
	}
	return m
}

func dispatch(name string) (tlEvt, error) {
	if name == "NEXT" {
		return tlNext{}, nil
	}
	return nil, studio.ErrUnknownEvent{Name: name}
}

func newTestServer() *studio.Server {
	srv := studio.NewServer("test")
	srv.Register(studio.Entry{
		Name:    "traffic-light",
		Summary: "demo",
		Build:   trafficLight().Describe,
		BuildLive: func() studio.LiveInstance {
			return studio.NewLiveActor(trafficLight(), dispatch, trafficLight().Describe)
		},
	})
	return srv
}

func TestLiveActor_StartAndSnapshot(t *testing.T) {
	la := studio.NewLiveActor(trafficLight(), dispatch, trafficLight().Describe)
	if err := la.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	snap := la.Snapshot()
	if snap.Path != "red" {
		t.Errorf("initial path: got %q want red", snap.Path)
	}
	if !strings.Contains(snap.ASCII, "▶ red") {
		t.Errorf("ASCII should highlight active red:\n%s", snap.ASCII)
	}
	evts := la.AvailableEvents()
	if len(evts) != 1 || evts[0] != "NEXT" {
		t.Errorf("available events: got %v want [NEXT]", evts)
	}
}

func TestLiveActor_SendAdvancesState(t *testing.T) {
	la := studio.NewLiveActor(trafficLight(), dispatch, trafficLight().Describe)
	_ = la.Start(context.Background())
	if err := la.SendEvent(context.Background(), "NEXT"); err != nil {
		t.Fatalf("SendEvent: %v", err)
	}
	if got := la.Snapshot().Path; got != "green" {
		t.Errorf("after NEXT: got %q want green", got)
	}
}

func TestLiveActor_UnknownEvent(t *testing.T) {
	la := studio.NewLiveActor(trafficLight(), dispatch, trafficLight().Describe)
	_ = la.Start(context.Background())
	err := la.SendEvent(context.Background(), "BOGUS")
	if err == nil {
		t.Fatal("expected error for unknown event")
	}
	var ue studio.ErrUnknownEvent
	if !strings.Contains(err.Error(), "BOGUS") {
		t.Errorf("error should name the event: %v", err)
	}
	_ = ue
}

func TestServer_HealthzAndIndex(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != 200 || rr.Body.String() != "ok" {
		t.Errorf("healthz: code=%d body=%q", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rr.Body.String(), "▶ simulate") {
		t.Error("index should show a simulate link")
	}
	if !strings.Contains(rr.Body.String(), "traffic-light") {
		t.Error("index should list traffic-light")
	}
}

func TestServer_SimSendFlow(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()

	// Reset first to get a clean session.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/sim/traffic-light/reset", nil))
	if rr.Code != 200 {
		t.Fatalf("reset: code=%d", rr.Code)
	}

	// Send NEXT.
	req := httptest.NewRequest(http.MethodPost, "/sim/traffic-light/send",
		strings.NewReader("event=NEXT"))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("send: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Path   string   `json:"path"`
		Events []string `json:"events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Path != "green" {
		t.Errorf("after send NEXT: path=%q want green", resp.Path)
	}
}

func TestServer_SimSendUnknownEvent400(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodPost, "/sim/traffic-light/send",
		strings.NewReader("event=BOGUS"))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("unknown event: code=%d want 400", rr.Code)
	}
}

func TestServer_DescribeJSON(t *testing.T) {
	srv := newTestServer()
	h := srv.Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/m/traffic-light/describe", nil))
	if rr.Code != 200 {
		t.Fatalf("describe: code=%d", rr.Code)
	}
	var d sc.MachineDescriptor
	if err := json.Unmarshal(rr.Body.Bytes(), &d); err != nil {
		t.Fatalf("descriptor unmarshal: %v", err)
	}
	if d.ID != "traffic-light" {
		t.Errorf("descriptor ID: got %q", d.ID)
	}
}
