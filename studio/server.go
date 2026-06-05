package studio

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	sc "github.com/arisros/fate"
)

// Entry is one registered machine. Build returns the static descriptor;
// BuildLive (optional) returns a fresh live actor for the simulator.
type Entry struct {
	Name      string
	Summary   string
	Build     func() sc.MachineDescriptor
	BuildLive func() LiveInstance // nil = static-only, no simulator
}

// Server is an embeddable statechart studio. Construct with NewServer,
// register machines with Register, then ListenAndServe or mount Handler.
type Server struct {
	title   string
	entries []Entry

	// live sessions keyed by machine name (one shared session per machine).
	sessions *sessionStore
}

// NewServer returns an empty studio. title appears in the page header.
func NewServer(title string) *Server {
	if title == "" {
		title = "fate studio"
	}
	return &Server{title: title, sessions: newSessionStore()}
}

// Register adds a machine. build is required (static view); buildLive is
// optional (interactive simulator). Returns the server for chaining.
func (s *Server) Register(e Entry) *Server {
	s.entries = append(s.entries, e)
	return s
}

func (s *Server) lookup(name string) (Entry, bool) {
	for _, e := range s.entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// Handler returns an http.Handler with all studio routes mounted.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/m/", s.handleMachine)
	mux.HandleFunc("/sim/", s.handleSimRoute)
	mux.HandleFunc("/assets/", s.handleAssets)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// ListenAndServe runs the studio on addr. WriteTimeout is intentionally
// unset (0) so the simulator's SSE connections can stay open.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return srv.ListenAndServe()
}

// ----- static handlers -----

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	var cards strings.Builder
	for _, e := range s.entries {
		sim := ""
		if e.BuildLive != nil {
			sim = fmt.Sprintf(`<a class="sim" href="/sim/%s">▶ simulate</a>`, html.EscapeString(e.Name))
		}
		fmt.Fprintf(&cards,
			`<div class="machine-card"><div class="mname">%s</div><div class="msum">%s</div>`+
				`<div class="mlinks"><a class="view" href="/m/%s">view</a>%s</div></div>`,
			html.EscapeString(e.Name), html.EscapeString(e.Summary),
			html.EscapeString(e.Name), sim)
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	renderShell(w, welcomeShell, html.EscapeString(s.title), cards.String())
}

func (s *Server) handleMachine(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/m/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.SplitN(rest, "/", 3)
	name := parts[0]
	entry, ok := s.lookup(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	descriptor := entry.Build()

	if len(parts) >= 2 && parts[1] == "graph" {
		// Resolved node/edge graph for the studio canvas (laid out by elkjs
		// client-side). Structure only — active highlight comes from SSE.
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(sc.RenderGraphJSON(descriptor))
		return
	}

	if len(parts) >= 2 && parts[1] == "describe" {
		w.Header().Set("content-type", "application/json")
		b, _ := json.MarshalIndent(descriptor, "", "  ")
		_, _ = w.Write(b)
		return
	}

	var statePath string
	if len(parts) >= 3 && parts[1] == "state" {
		statePath = parts[2]
	}

	ascii := sc.RenderASCII(descriptor, sc.RenderOptions{Highlight: highlightForActivePath(statePath)})
	transitions := ""
	if statePath != "" {
		transitions = sc.RenderTransitions(descriptor, statePath)
	}

	simLink := ""
	if entry.BuildLive != nil {
		simLink = fmt.Sprintf(` | <a href="/sim/%s">▶ simulate</a>`, html.EscapeString(name))
	}

	w.Header().Set("content-type", "text/html; charset=utf-8")
	renderShell(w, pageShell, html.EscapeString(s.title)+" — "+html.EscapeString(name),
		fmt.Sprintf(`<h1>%s</h1>
<p><a href="/">&larr; index</a> | <a href="/m/%s/describe">JSON descriptor</a>%s</p>
<h2>State diagram</h2>
<pre class="diagram">%s</pre>
%s
<h2>Inspect state</h2>
<form onsubmit="window.location='/m/%s/state/'+document.getElementById('p').value;return false">
  <label>dot-path: <input id="p" placeholder="e.g. active.main"></label>
  <button type="submit">view</button>
</form>`,
			html.EscapeString(name), html.EscapeString(name), simLink,
			html.EscapeString(ascii),
			transitionBlock(transitions, statePath),
			html.EscapeString(name),
		))
}

func transitionBlock(rendered, path string) string {
	if rendered == "" {
		return ""
	}
	return fmt.Sprintf(`<h2>Transitions from <code>%s</code></h2><pre class="sidebar">%s</pre>`,
		html.EscapeString(path), html.EscapeString(rendered))
}
