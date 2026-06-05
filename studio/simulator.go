package studio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// sessionTTL is how long an idle session survives before the reaper evicts it.
const sessionTTL = 30 * time.Minute

// maxHistory caps the per-session undo stack (defensive — prevents unbounded
// memory if someone scripts thousands of events).
const maxHistory = 500

// ----- session -----

// session is one user's live simulation of one machine. Guarded by mu — all
// access to live / history / events / subs must hold it.
type session struct {
	build func() LiveInstance

	mu       sync.Mutex
	live     LiveInstance
	subs     []chan LiveSnapshot
	history  [][]byte // snapshot bytes captured *before* each applied event
	events   []string // event names, parallel to history
	lastSeen time.Time
}

func (s *session) touch() {
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
}

// snapshot returns the current LiveSnapshot under lock.
func (s *session) snapshot() LiveSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live.Snapshot()
}

func (s *session) availableEvents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live.AvailableEvents()
}

// applyEvent pushes the pre-event snapshot to history, then dispatches. On
// dispatch error the history entry is rolled back.
func (s *session) applyEvent(ctx context.Context, ev string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, perr := s.live.Persist()
	if perr == nil {
		if len(s.history) >= maxHistory {
			s.history = s.history[1:]
			s.events = s.events[1:]
		}
		s.history = append(s.history, before)
		s.events = append(s.events, ev)
	}
	if err := s.live.SendEvent(ctx, ev); err != nil {
		if perr == nil { // roll back the history push
			s.history = s.history[:len(s.history)-1]
			s.events = s.events[:len(s.events)-1]
		}
		return err
	}
	return nil
}

// applyEffect captures the pre-effect snapshot (for undo/timeline), runs fn,
// and rolls back the history push if fn fails. Used for timer/invocation
// effects, which advance the machine like events but aren't user events.
func (s *session) applyEffect(label string, fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	before, perr := s.live.Persist()
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

func (s *session) fireTimer(id string) error {
	return s.applyEffect("⏲ after", func() error { return s.live.FireTimer(id) })
}

func (s *session) resolveInvocation(id, output string) error {
	return s.applyEffect("✓ "+id, func() error { return s.live.ResolveInvocation(id, output) })
}

func (s *session) rejectInvocation(id, errMsg string) error {
	return s.applyEffect("✗ "+id, func() error { return s.live.RejectInvocation(id, errMsg) })
}

// undo pops the last event and restores the prior snapshot. Returns false if
// there is nothing to undo.
func (s *session) undo() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.history) == 0 {
		return false, nil
	}
	snap := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	s.events = s.events[:len(s.events)-1]
	return true, s.live.Restore(snap)
}

// reset rebuilds a fresh actor in place (keeping SSE subscribers attached) and
// clears history.
func (s *session) reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	live := s.build()
	if err := live.Start(context.Background()); err != nil {
		return err
	}
	s.live = live
	s.history = nil
	s.events = nil
	return nil
}

// importSnapshot restores from external snapshot bytes and clears history.
func (s *session) importSnapshot(b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.live.Restore(b); err != nil {
		return err
	}
	s.history = nil
	s.events = nil
	return nil
}

func (s *session) timeline() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.events))
	copy(out, s.events)
	return out
}

func (s *session) persist() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.live.Persist()
}

func (s *session) broadcast() {
	snap := s.snapshot()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- snap:
		default:
		}
	}
}

func (s *session) subscribe() (chan LiveSnapshot, func()) {
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

// ----- session store (per machine+token) -----

type sessionStore struct {
	mu sync.Mutex
	m  map[string]*session // key: machine + "|" + token
}

func newSessionStore() *sessionStore {
	st := &sessionStore{m: map[string]*session{}}
	go st.reap()
	return st
}

func (st *sessionStore) getOrCreate(key string, build func() LiveInstance) (*session, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if s, ok := st.m[key]; ok {
		s.lastSeen = time.Now()
		return s, nil
	}
	if build == nil {
		return nil, fmt.Errorf("no live simulator registered")
	}
	live := build()
	if err := live.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("actor start: %w", err)
	}
	s := &session{build: build, live: live, lastSeen: time.Now()}
	st.m[key] = s
	return s, nil
}

// reap evicts idle sessions so actors don't accumulate.
func (st *sessionStore) reap() {
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

// ----- token / cookie -----

const sessionCookie = "fate_sid"

// tokenFor reads the session token from the cookie, minting + setting one when
// absent. Per-browser isolation: each token gets its own actor per machine.
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

// ----- routing -----

func (s *Server) handleSimRoute(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/sim/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	name := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	switch sub {
	case "":
		s.handleSimPage(w, r, name)
	case "stream":
		s.handleSimStream(w, r, name)
	case "send":
		s.handleSimSend(w, r, name)
	case "timer":
		s.handleSimTimer(w, r, name)
	case "invoke":
		s.handleSimInvoke(w, r, name)
	case "reset":
		s.handleSimReset(w, r, name)
	case "undo":
		s.handleSimUndo(w, r, name)
	case "import":
		s.handleSimImport(w, r, name)
	case "timeline":
		s.handleSimTimeline(w, r, name)
	case "export":
		s.handleSimExport(w, r, name)
	default:
		http.NotFound(w, r)
	}
}

// sessionFor resolves (or creates) the per-browser session, minting the cookie
// when needed.
func (s *Server) sessionFor(w http.ResponseWriter, r *http.Request, name string) (*session, error) {
	entry, ok := s.lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown machine %q", name)
	}
	token := tokenFor(w, r)
	return s.sessions.getOrCreate(name+"|"+token, entry.BuildLive)
}

// snapResponse is the JSON shape returned by send/reset/undo/import.
type snapResponse struct {
	LiveSnapshot
	Events []string `json:"events"`
}

func writeSnapResponse(w http.ResponseWriter, sess *session) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(snapResponse{
		LiveSnapshot: sess.snapshot(),
		Events:       sess.availableEvents(),
	})
}

func (s *Server) handleSimPage(w http.ResponseWriter, r *http.Request, name string) {
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	evJSON, _ := json.Marshal(sess.availableEvents())
	nameJSON, _ := json.Marshal(name)
	globals := "window.FATE_MACHINE=" + string(nameJSON) + ";window.FATE_EVENTS=" + string(evJSON) + ";"

	w.Header().Set("content-type", "text/html; charset=utf-8")
	renderShell(w, simShell,
		html.EscapeString(s.title)+" — "+html.EscapeString(name),
		html.EscapeString(name),
		html.EscapeString(name),
		globals,
	)
}

func (s *Server) handleSimStream(w http.ResponseWriter, r *http.Request, name string) {
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")

	if err := writeSSE(w, sess.snapshot()); err != nil {
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

func (s *Server) handleSimSend(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	_ = r.ParseForm()
	evtName := r.FormValue("event")
	if evtName == "" {
		http.Error(w, "missing 'event' field", http.StatusBadRequest)
		return
	}
	if err := sess.applyEvent(r.Context(), evtName); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sess.broadcast()
	writeSnapResponse(w, sess)
}

func (s *Server) handleSimTimer(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
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
	sess.broadcast()
	writeSnapResponse(w, sess)
}

func (s *Server) handleSimInvoke(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	_ = r.ParseForm()
	id := r.FormValue("id")
	if id == "" {
		http.Error(w, "missing 'id' field", http.StatusBadRequest)
		return
	}
	if r.FormValue("action") == "reject" {
		err = sess.rejectInvocation(id, r.FormValue("error"))
	} else {
		err = sess.resolveInvocation(id, r.FormValue("output"))
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sess.broadcast()
	writeSnapResponse(w, sess)
}

func (s *Server) handleSimReset(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := sess.reset(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sess.broadcast()
	writeSnapResponse(w, sess)
}

func (s *Server) handleSimUndo(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	ok, err := sess.undo()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "nothing to undo", http.StatusBadRequest)
		return
	}
	sess.broadcast()
	writeSnapResponse(w, sess)
}

func (s *Server) handleSimImport(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := sess.importSnapshot(body); err != nil {
		http.Error(w, "import: "+err.Error(), http.StatusBadRequest)
		return
	}
	sess.broadcast()
	writeSnapResponse(w, sess)
}

func (s *Server) handleSimTimeline(w http.ResponseWriter, r *http.Request, name string) {
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": sess.timeline()})
}

func (s *Server) handleSimExport(w http.ResponseWriter, r *http.Request, name string) {
	sess, err := s.sessionFor(w, r, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	b, err := sess.persist()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.Header().Set("content-disposition", fmt.Sprintf(`attachment; filename="%s-snapshot.json"`, name))
	_, _ = w.Write(b)
}
