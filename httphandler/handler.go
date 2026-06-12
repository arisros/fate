// Package httphandler exposes a fate [fate.Actor] as an HTTP simulator API —
// the same wire protocol as fate-studio's /sim/{name}/* endpoints. Services
// mount it on their own router; fate-studio connects via ProxyURL without any
// Go import coupling.
//
// # Usage
//
//	h := httphandler.New(machine, dispatchFn)
//	mux.Handle("/fsm/survey/", http.StripPrefix("/fsm/survey", h))
//
// # Endpoints
//
//	GET  /stream     — SSE stream of LiveSnapshot (text/event-stream)
//	POST /send       — body: event=NAME → returns snapResponse JSON
//	POST /reset      — returns snapResponse JSON
//	POST /undo       — returns snapResponse JSON, 400 if nothing to undo
//	GET  /timeline   — returns {"events":[...]}
//	GET  /export     — returns persisted snapshot bytes (application/json)
//	POST /import     — body: snapshot bytes → returns snapResponse JSON
//	POST /timer      — body: id=TIMER_ID → returns snapResponse JSON
//	POST /invoke     — body: id=…, action=resolve|reject, output=…, error=…
//
// # Session model
//
// Each browser gets its own actor instance, isolated by a session cookie
// (fate_sid). Sessions time out after 30 minutes of inactivity and are reaped
// by a background goroutine started on first New() call.
package httphandler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arisros/fate"
	"github.com/arisros/fate/render"
)

const (
	sessionCookie = "fate_sid"
	sessionTTL    = 30 * time.Minute
	maxHistory    = 500
)

// LiveSnapshot is the SSE payload pushed after every state change. Its shape
// is identical to fate-studio's LiveSnapshot so that fate-studio's ProxyURL
// mode can forward it transparently.
type LiveSnapshot struct {
	Path        string           `json:"path"`
	Context     json.RawMessage  `json:"context"`
	Status      fate.ActorStatus `json:"status"`
	ASCII       string           `json:"ascii"`
	Timers      []timerInfo      `json:"timers,omitempty"`
	Invocations []invokeInfo     `json:"invocations,omitempty"`
}

type timerInfo struct {
	ID    string `json:"id"`
	Delay string `json:"delay"`
}

type invokeInfo struct {
	ID  string `json:"id"`
	Src string `json:"src"`
}

// snapResponse is returned by send/reset/undo/import.
type snapResponse struct {
	LiveSnapshot
	Events []string `json:"events"`
}

// Handler is an http.Handler that wraps a session store for one machine.
// Obtain one via [New].
type Handler[Ctx any, Evt any] struct {
	machine  *fate.Machine[Ctx, Evt]
	dispatch func(string) (Evt, error)
	desc     fate.MachineDescriptor
	store    *sessionStore[Ctx, Evt]
}

// New returns an http.Handler that exposes machine as an interactive simulator
// API. dispatch maps event-name strings to typed events; it should return
// ErrUnknownEvent for unrecognised names (surfaces as HTTP 400).
func New[Ctx any, Evt any](
	m *fate.Machine[Ctx, Evt],
	dispatch func(string) (Evt, error),
) *Handler[Ctx, Evt] {
	h := &Handler[Ctx, Evt]{
		machine:  m,
		dispatch: dispatch,
		desc:     m.Describe(),
		store:    newSessionStore[Ctx, Evt](),
	}
	return h
}

// ErrUnknownEvent is the canonical error a dispatch function returns for an
// unrecognised event name.
type ErrUnknownEvent struct{ Name string }

func (e ErrUnknownEvent) Error() string { return fmt.Sprintf("unknown event %q", e.Name) }

// ServeHTTP routes to the appropriate sub-handler based on the final path
// segment. The handler is designed to be mounted with http.StripPrefix so that
// /stream, /send, etc. are at the root.
func (h *Handler[Ctx, Evt]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/")
	sub = strings.TrimSuffix(sub, "/")
	switch sub {
	case "stream":
		h.handleStream(w, r)
	case "send":
		h.handleSend(w, r)
	case "timer":
		h.handleTimer(w, r)
	case "invoke":
		h.handleInvoke(w, r)
	case "reset":
		h.handleReset(w, r)
	case "undo":
		h.handleUndo(w, r)
	case "import":
		h.handleImport(w, r)
	case "timeline":
		h.handleTimeline(w, r)
	case "export":
		h.handleExport(w, r)
	default:
		http.NotFound(w, r)
	}
}

// ----- snapshot building -----

func (h *Handler[Ctx, Evt]) buildSnapshot(a *fate.Actor[Ctx, Evt]) LiveSnapshot {
	snap := a.Snapshot()
	ctxBytes, _ := json.Marshal(snap.Context)
	activePath := snap.Value.Path()

	var hl map[string]rune
	if activePath != "" {
		hl = map[string]rune{}
		for _, region := range strings.Split(activePath, " | ") {
			hl[strings.TrimSpace(region)] = '▶'
		}
	}
	asciiStr := render.ASCII(h.desc, render.Options{Highlight: hl})

	pts := a.PendingTimers()
	timers := make([]timerInfo, 0, len(pts))
	for _, p := range pts {
		timers = append(timers, timerInfo{ID: string(p.ID), Delay: p.Delay.String()})
	}

	pis := a.PendingInvocations()
	invokes := make([]invokeInfo, 0, len(pis))
	for _, p := range pis {
		invokes = append(invokes, invokeInfo{ID: string(p.ID), Src: p.Src})
	}

	return LiveSnapshot{
		Path:        activePath,
		Context:     ctxBytes,
		Status:      snap.Status,
		ASCII:       asciiStr,
		Timers:      timers,
		Invocations: invokes,
	}
}

func (h *Handler[Ctx, Evt]) availableEvents(a *fate.Actor[Ctx, Evt]) []string {
	path := a.Snapshot().Value.Path()
	seen := map[string]struct{}{}
	var evts []string
	for _, region := range strings.Split(path, " | ") {
		node, ok := descriptorNodeAt(h.desc, strings.TrimSpace(region))
		if !ok {
			continue
		}
		for k := range node.On {
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			evts = append(evts, k)
		}
	}
	sort.Strings(evts)
	return evts
}

func (h *Handler[Ctx, Evt]) writeSnap(w http.ResponseWriter, s *session[Ctx, Evt]) {
	w.Header().Set("content-type", "application/json")
	s.mu.Lock()
	snap := h.buildSnapshot(s.actor)
	evts := h.availableEvents(s.actor)
	s.mu.Unlock()
	_ = json.NewEncoder(w).Encode(snapResponse{LiveSnapshot: snap, Events: evts})
}

// ----- session / token -----

func tokenFor(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	return tok
}

func (h *Handler[Ctx, Evt]) sessionFor(w http.ResponseWriter, r *http.Request) *session[Ctx, Evt] {
	tok := tokenFor(w, r)
	return h.store.getOrCreate(tok, func() *fate.Actor[Ctx, Evt] {
		a := fate.NewActor(h.machine)
		_ = a.Start(context.Background())
		return a
	})
}

// ----- handlers -----

func (h *Handler[Ctx, Evt]) handleStream(w http.ResponseWriter, r *http.Request) {
	sess := h.sessionFor(w, r)
	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")

	sess.mu.Lock()
	initial := h.buildSnapshot(sess.actor)
	sess.mu.Unlock()
	if err := writeSSE(w, initial); err != nil {
		return
	}

	ch, unsub := sess.subscribe()
	defer unsub()

	ctx := r.Context()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case snap, open := <-ch:
			if !open {
				return
			}
			if err := writeSSE(w, snap); err != nil {
				return
			}
		case <-keepalive.C:
			sess.touch()
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler[Ctx, Evt]) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess := h.sessionFor(w, r)
	_ = r.ParseForm()
	evtName := r.FormValue("event")
	if evtName == "" {
		http.Error(w, "missing 'event' field", http.StatusBadRequest)
		return
	}
	if err := sess.applyEvent(r.Context(), evtName, h.dispatch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.broadcastTo(sess)
	h.writeSnap(w, sess)
}

func (h *Handler[Ctx, Evt]) handleTimer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess := h.sessionFor(w, r)
	_ = r.ParseForm()
	id := r.FormValue("id")
	if id == "" {
		http.Error(w, "missing 'id' field", http.StatusBadRequest)
		return
	}
	if err := sess.fireTimer(id); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.broadcastTo(sess)
	h.writeSnap(w, sess)
}

func (h *Handler[Ctx, Evt]) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess := h.sessionFor(w, r)
	_ = r.ParseForm()
	id := r.FormValue("id")
	if id == "" {
		http.Error(w, "missing 'id' field", http.StatusBadRequest)
		return
	}
	var err error
	if r.FormValue("action") == "reject" {
		err = sess.rejectInvocation(id, r.FormValue("error"))
	} else {
		err = sess.resolveInvocation(id, r.FormValue("output"))
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.broadcastTo(sess)
	h.writeSnap(w, sess)
}

func (h *Handler[Ctx, Evt]) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess := h.sessionFor(w, r)
	sess.reset(h.machine)
	h.broadcastTo(sess)
	h.writeSnap(w, sess)
}

func (h *Handler[Ctx, Evt]) handleUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess := h.sessionFor(w, r)
	ok, err := sess.undo(h.machine)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "nothing to undo", http.StatusBadRequest)
		return
	}
	h.broadcastTo(sess)
	h.writeSnap(w, sess)
}

func (h *Handler[Ctx, Evt]) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess := h.sessionFor(w, r)
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := sess.importSnapshot(h.machine, body); err != nil {
		http.Error(w, "import: "+err.Error(), http.StatusBadRequest)
		return
	}
	h.broadcastTo(sess)
	h.writeSnap(w, sess)
}

func (h *Handler[Ctx, Evt]) handleTimeline(w http.ResponseWriter, r *http.Request) {
	sess := h.sessionFor(w, r)
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": sess.timeline()})
}

func (h *Handler[Ctx, Evt]) handleExport(w http.ResponseWriter, r *http.Request) {
	sess := h.sessionFor(w, r)
	b, err := sess.persist()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	_, _ = w.Write(b)
}

// broadcastTo sends the current snapshot to all SSE subscribers of sess.
func (h *Handler[Ctx, Evt]) broadcastTo(sess *session[Ctx, Evt]) {
	sess.mu.Lock()
	snap := h.buildSnapshot(sess.actor)
	subs := sess.subs
	sess.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- snap:
		default:
		}
	}
}

// ----- SSE helper -----

func writeSSE(w io.Writer, snap LiveSnapshot) error {
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// ----- descriptor walk -----

func descriptorNodeAt(d fate.MachineDescriptor, path string) (fate.StateNodeDescriptor, bool) {
	if path == "" {
		return fate.StateNodeDescriptor{}, false
	}
	segs := strings.Split(path, ".")
	cur, ok := d.States[segs[0]]
	if !ok {
		return fate.StateNodeDescriptor{}, false
	}
	for _, s := range segs[1:] {
		next, ok := cur.States[s]
		if !ok {
			return fate.StateNodeDescriptor{}, false
		}
		cur = next
	}
	return cur, true
}

// ----- session -----

type session[Ctx any, Evt any] struct {
	mu       sync.Mutex
	actor    *fate.Actor[Ctx, Evt]
	subs     []chan LiveSnapshot
	history  [][]byte
	events   []string
	lastSeen time.Time
}

func (s *session[Ctx, Evt]) touch() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}

func (s *session[Ctx, Evt]) applyEvent(ctx context.Context, name string, dispatch func(string) (Evt, error)) error {
	evt, err := dispatch(name)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	before, perr := s.actor.Persist()
	if perr == nil {
		if len(s.history) >= maxHistory {
			s.history = s.history[1:]
			s.events = s.events[1:]
		}
		s.history = append(s.history, before)
		s.events = append(s.events, name)
	}
	if err := s.actor.Send(ctx, evt); err != nil {
		if perr == nil {
			s.history = s.history[:len(s.history)-1]
			s.events = s.events[:len(s.events)-1]
		}
		return err
	}
	return nil
}

func (s *session[Ctx, Evt]) applyEffect(label string, fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, perr := s.actor.Persist()
	if perr == nil {
		if len(s.history) >= maxHistory {
			s.history = s.history[1:]
			s.events = s.events[1:]
		}
		s.history = append(s.history, before)
		s.events = append(s.events, label)
	}
	if err := fn(); err != nil {
		if perr == nil {
			s.history = s.history[:len(s.history)-1]
			s.events = s.events[:len(s.events)-1]
		}
		return err
	}
	return nil
}

func (s *session[Ctx, Evt]) fireTimer(id string) error {
	return s.applyEffect("⏲ after", func() error {
		s.actor.FireTimer(fate.TimerID(id))
		return nil
	})
}

func (s *session[Ctx, Evt]) resolveInvocation(id, outputJSON string) error {
	return s.applyEffect("✓ "+id, func() error {
		var out interface{}
		if t := strings.TrimSpace(outputJSON); t != "" {
			if err := json.Unmarshal([]byte(t), &out); err != nil {
				return fmt.Errorf("output is not valid JSON: %w", err)
			}
		}
		s.actor.ResolveInvocation(fate.InvokeID(id), out)
		return nil
	})
}

func (s *session[Ctx, Evt]) rejectInvocation(id, errMsg string) error {
	return s.applyEffect("✗ "+id, func() error {
		if errMsg == "" {
			errMsg = "rejected"
		}
		s.actor.RejectInvocation(fate.InvokeID(id), errors.New(errMsg))
		return nil
	})
}

func (s *session[Ctx, Evt]) reset(m *fate.Machine[Ctx, Evt]) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := fate.NewActor(m)
	_ = a.Start(context.Background())
	s.actor = a
	s.history = nil
	s.events = nil
}

func (s *session[Ctx, Evt]) undo(m *fate.Machine[Ctx, Evt]) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return false, nil
	}
	snap := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	s.events = s.events[:len(s.events)-1]
	a, err := fate.NewActorFromSnapshot(m, snap)
	if err != nil {
		return false, err
	}
	s.actor = a
	return true, nil
}

func (s *session[Ctx, Evt]) importSnapshot(m *fate.Machine[Ctx, Evt], data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := fate.NewActorFromSnapshot(m, data)
	if err != nil {
		return err
	}
	s.actor = a
	s.history = nil
	s.events = nil
	return nil
}

func (s *session[Ctx, Evt]) timeline() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.events))
	copy(out, s.events)
	return out
}

func (s *session[Ctx, Evt]) persist() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.actor.Persist()
}

func (s *session[Ctx, Evt]) subscribe() (chan LiveSnapshot, func()) {
	ch := make(chan LiveSnapshot, 4)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, c := range s.subs {
			if c == ch {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
}

// ----- session store -----

type sessionStore[Ctx any, Evt any] struct {
	mu sync.Mutex
	m  map[string]*session[Ctx, Evt]
}

func newSessionStore[Ctx any, Evt any]() *sessionStore[Ctx, Evt] {
	st := &sessionStore[Ctx, Evt]{m: map[string]*session[Ctx, Evt]{}}
	go st.reap()
	return st
}

func (st *sessionStore[Ctx, Evt]) getOrCreate(token string, build func() *fate.Actor[Ctx, Evt]) *session[Ctx, Evt] {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s, ok := st.m[token]; ok {
		s.lastSeen = time.Now()
		return s
	}
	s := &session[Ctx, Evt]{actor: build(), lastSeen: time.Now()}
	st.m[token] = s
	return s
}

func (st *sessionStore[Ctx, Evt]) reap() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-sessionTTL)
		st.mu.Lock()
		for k, s := range st.m {
			s.mu.Lock()
			idle := s.lastSeen.Before(cutoff)
			s.mu.Unlock()
			if idle {
				delete(st.m, k)
			}
		}
		st.mu.Unlock()
	}
}
