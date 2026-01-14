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

//go:embed assets/*
var assetsFS embed.FS

var assetsSub, _ = fs.Sub(assetsFS, "assets")

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
	sort.Strings(names)
	for _, n := range names {
		if b, err := fs.ReadFile(assetsSub, n); err == nil {
			h.Write([]byte(n))
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

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
	w.Header().Set("cache-control", "public, max-age=31536000, immutable")

	if rs, ok := f.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, time.Time{}, rs)
		return
	}
	data, _ := fs.ReadFile(assetsSub, name)
	_, _ = w.Write(data)
}
