package api

import (
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// StaticHandler serves a web client build (Godot HTML5 export) from dir.
// If "<file>.gz" exists next to a requested file and the client accepts
// gzip, the pre-compressed copy is served — the engine .wasm is ~35 MB raw
// and ~9 MB gzipped, which matters a lot on a phone. build_web.sh produces
// the .gz files.
func StaticHandler(dir string) http.Handler {
	fs := http.Dir(dir)
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		if clean == "/" {
			clean = "/index.html"
		}
		w.Header().Set("Cache-Control", "no-cache")

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			gzPath := filepath.Join(dir, filepath.FromSlash(clean)) + ".gz"
			if f, err := os.Open(gzPath); err == nil {
				defer f.Close()
				if st, err := f.Stat(); err == nil && !st.IsDir() {
					if ct := mime.TypeByExtension(path.Ext(clean)); ct != "" {
						w.Header().Set("Content-Type", ct)
					} else {
						w.Header().Set("Content-Type", "application/octet-stream")
					}
					w.Header().Set("Content-Encoding", "gzip")
					w.Header().Set("Vary", "Accept-Encoding")
					http.ServeContent(w, r, clean, st.ModTime(), f)
					return
				}
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
