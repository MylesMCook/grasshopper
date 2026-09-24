package gomcp

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

// The page is public so a browser can open it without putting a bearer token
// in a URL or cookie. It contains no memory data; the API still requires auth.
//
//go:embed visualizer/index.html visualizer/app.js visualizer/style.css visualizer/*.woff2
var visualizerFiles embed.FS

func visualizerAsset(w http.ResponseWriter, r *http.Request) bool {
	var filename, contentType string
	switch r.URL.Path {
	case "/visualizer", "/visualizer/":
		filename, contentType = "index.html", "text/html; charset=utf-8"
	case "/visualizer/app.js":
		filename, contentType = "app.js", "text/javascript; charset=utf-8"
	case "/visualizer/style.css":
		filename, contentType = "style.css", "text/css; charset=utf-8"
	case "/visualizer/newsreader-latin.woff2":
		filename, contentType = "newsreader-latin.woff2", "font/woff2"
	case "/visualizer/geist-mono-latin.woff2":
		filename, contentType = "geist-mono-latin.woff2", "font/woff2"
	default:
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; font-src 'self'; img-src data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	contents, err := visualizerFiles.ReadFile("visualizer/" + filename)
	if err != nil {
		http.Error(w, "visualizer unavailable", http.StatusInternalServerError)
		return true
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(contents)
	return true
}

func visualizerContext(store *gomemory.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		scope := gomemory.Scope{}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&scope); err != nil {
			http.Error(w, "invalid scope", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			http.Error(w, "invalid scope", http.StatusBadRequest)
			return
		}
		if _, err := scope.Key(); err != nil || scope.Legacy {
			http.Error(w, "invalid scope", http.StatusBadRequest)
			return
		}
		page, err := store.Context(r.Context(), scope, 32768)
		if err != nil {
			http.Error(w, "context unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(page)
	}
}
