package studio

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Embedded static assets served at /assets/*. elk.bundled.js is vendored
// (elkjs@0.9.3) so the studio is fully self-contained — no CDN request at
// runtime, which matters for internal/offline use.
//
//go:embed assets/*
var assetsFS embed.FS

// assetsSub is the assets/ subtree rooted so paths are clean ("/assets/app.css"
// → "app.css").
var assetsSub, _ = fs.Sub(assetsFS, "assets")

// assetVersion is a short content hash over every embedded asset. It is appended
// to asset URLs (?v=<hash>) so a redeploy with changed assets yields fresh URLs —
// busting any CDN/browser cache automatically without manual purge. The token
// "__ASSETVER__" in the HTML shells is replaced with this at render time.
var assetVersion = computeAssetVersion()

func computeAssetVersion() string {
	h := sha256.New()
	names := []string{}
	_ = fs.WalkDir(assetsSub, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return nil
	})
	sort.Strings(names) // deterministic across runs
	for _, n := range names {
		if b, err := fs.ReadFile(assetsSub, n); err == nil {
			h.Write([]byte(n))
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

// handleAssets serves an embedded asset. Long cache headers because the files
// are versioned with the binary.
func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/assets/")
	if name == "" || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	f, err := assetsSub.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	switch {
	case strings.HasSuffix(name, ".js"):
		w.Header().Set("content-type", "application/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".css"):
		w.Header().Set("content-type", "text/css; charset=utf-8")
	}
	// URLs carry a content hash (?v=…), so a given URL's bytes never change —
	// safe to cache immutably. New content ⇒ new URL ⇒ automatic cache bust.
	w.Header().Set("cache-control", "public, max-age=31536000, immutable")

	// embed.FS files implement io.Seeker, so ServeContent gives range support.
	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	data, _ := fs.ReadFile(assetsSub, name)
	_, _ = w.Write(data)
}
