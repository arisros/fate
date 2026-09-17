// Command fate-site serves the built documentation site.
//
// It is a static file server with three jobs beyond handing back bytes:
// resolving VitePress clean URLs to their .html files, redirecting the page
// paths the previous site published, and answering a health probe. It uses only
// the standard library, so it inherits the engine module's empty dependency
// graph.
//
// Configuration is by environment variable:
//
//	FATE_SITE_ADDR   listen address           (default ":8090")
//	FATE_SITE_ROOT   directory to serve from  (default "/public")
package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// redirects maps page paths published by the previous site to their current
// homes, so links already in the wild keep working. Values that start with a
// scheme are sent as-is.
var redirects = map[string]string{
	"/guide/what-is-fate":        "/",
	"/guide/background":          "/concepts",
	"/guide/concepts":            "/concepts",
	"/guide/install":             "/guide/getting-started",
	"/guide/how-it-works":        "/guide/effects-and-adapters",
	"/guide/how-the-chart-works": "/cli",
	"/guide/studio":              "https://fate-studio.arisjirat.com",
	"/examples":                  "https://github.com/arisros/fate/tree/main/examples",
	"/changelog":                 "https://github.com/arisros/fate/blob/main/CHANGELOG.md",
}

func main() {
	addr := env("FATE_SITE_ADDR", ":8090")
	root := env("FATE_SITE_ROOT", "/public")

	if _, err := os.Stat(filepath.Join(root, "index.html")); err != nil {
		log.Fatalf("fate-site: %s does not look like a built site: %v", root, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/", &site{root: root})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("fate-site: serving %s on %s", root, addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("fate-site: %v", err)
	}
}

type site struct{ root string }

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// path.Clean on a rooted path resolves any "..", so the join below cannot
	// escape the root.
	upath := path.Clean("/" + r.URL.Path)

	if to, ok := redirects[strings.TrimSuffix(upath, "/")]; ok {
		http.Redirect(w, r, to, http.StatusMovedPermanently)
		return
	}

	name, ok := s.resolve(upath)
	if !ok {
		s.notFound(w, r)
		return
	}

	if strings.HasPrefix(upath, "/assets/") {
		// VitePress fingerprints everything under /assets/.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFile(w, r, name)
}

// resolve maps a URL path to a file on disk, trying the VitePress clean-URL
// forms in turn: the path itself, then <path>.html, then <path>/index.html.
func (s *site) resolve(upath string) (string, bool) {
	base := filepath.Join(s.root, filepath.FromSlash(upath))

	candidates := []string{base + ".html", base, filepath.Join(base, "index.html")}
	if strings.HasSuffix(upath, "/") {
		candidates = []string{filepath.Join(base, "index.html")}
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() {
			return c, true
		}
	}
	return "", false
}

func (s *site) notFound(w http.ResponseWriter, r *http.Request) {
	page := filepath.Join(s.root, "404.html")
	body, err := os.ReadFile(page)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(body)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
