package gomcp

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

// The page is public so a browser can open it without putting a bearer token
// in a URL or cookie. It contains no memory data; the API still requires auth.
//
//go:embed visualizer/index.html visualizer/app.js visualizer/theme.js visualizer/style.css visualizer/*.woff2
var visualizerFiles embed.FS

func visualizerAsset(w http.ResponseWriter, r *http.Request, styleHashes []string) bool {
	var filename, contentType string
	switch r.URL.Path {
	case "/visualizer", "/visualizer/":
		filename, contentType = "index.html", "text/html; charset=utf-8"
	case "/visualizer/app.js":
		filename, contentType = "app.js", "text/javascript; charset=utf-8"
	case "/visualizer/theme.js":
		filename, contentType = "theme.js", "text/javascript; charset=utf-8"
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
	w.Header().Set("Content-Security-Policy", staticCSP(styleHashes, filename == "index.html"))
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

// Owner reads never write or re-embed stored records. Search shares the MCP
// inference gate; a failed query embedding falls back to disclosed lexical recall.
func visualizerSearch(backend Backend, embedQuery func(context.Context, string) ([]float32, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Scope gomemory.BrowseScope `json:"scope"`
			Query string               `json:"query"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if _, err := input.Scope.Key(); err != nil || input.Scope.Legacy || strings.TrimSpace(input.Query) == "" || len(input.Query) > 4096 {
			http.Error(w, "invalid search", http.StatusBadRequest)
			return
		}
		vector, err := embedQuery(r.Context(), input.Query)
		if err != nil {
			vector = nil
		}
		page, devices, projects, err := backend.Store.BrowseSearch(r.Context(), input.Scope, input.Query, vector, backend.Model, 100, 32768)
		var titles map[int64]string
		if err == nil {
			devices, err = visualizerDevices(r.Context(), backend.Store, devices)
		}
		if err == nil {
			titles, err = omittedTitles(r.Context(), backend.Store, page)
		}
		if err != nil {
			http.Error(w, "search unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(struct {
			gomemory.Page
			Devices       []string         `json:"devices"`
			Projects      []string         `json:"projects"`
			OmittedTitles map[int64]string `json:"omitted_titles"`
			SemanticReady bool             `json:"semantic_ready"`
		}{page, devices, projects, titles, vector != nil})
	}
}

func visualizerRecord(store *gomemory.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Scope    gomemory.BrowseScope `json:"scope"`
			ID       int64                `json:"id"`
			Revision *int64               `json:"revision,omitempty"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if _, err := input.Scope.Key(); err != nil || input.Scope.Legacy || input.ID < 1 || (input.Revision != nil && *input.Revision < 1) {
			http.Error(w, "invalid record request", http.StatusBadRequest)
			return
		}
		record, err := store.BrowseGet(r.Context(), input.Scope, input.ID, input.Revision)
		if err != nil {
			http.Error(w, "record unavailable", http.StatusServiceUnavailable)
			return
		}
		if record == nil {
			http.Error(w, "memory not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(record)
	}
}

func decodeVisualizerRead(w http.ResponseWriter, r *http.Request, input any) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}
	return true
}

func staticCSP(styleHashes []string, document bool) string {
	styleSource := "'self'"
	if document {
		for _, hash := range styleHashes {
			styleSource += " '" + hash + "'"
		}
	}
	return "default-src 'none'; script-src 'self'; style-src " + styleSource + "; font-src 'self'; img-src data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}

// Device choices include active connections even before they have scoped
// memories. Saved-memory choices retain their project/platform filtering;
// connection names carry no such scope and do not widen record retrieval.
func visualizerDevices(ctx context.Context, store *gomemory.Writer, devices []string) ([]string, error) {
	tokens, err := store.ListClientTokens(ctx)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		if token.RevokedAt == nil {
			devices = append(devices, token.Device)
		}
	}
	slices.Sort(devices)
	return slices.Compact(devices), nil
}

func visualizerContext(store *gomemory.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		scope := gomemory.BrowseScope{}
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
		page, devices, projects, err := store.BrowseContext(r.Context(), scope, 32768)
		var titles map[int64]string
		if err == nil {
			devices, err = visualizerDevices(r.Context(), store, devices)
		}
		if err == nil {
			titles, err = omittedTitles(r.Context(), store, page)
		}
		if err != nil {
			http.Error(w, "context unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(struct {
			gomemory.Page
			Devices       []string         `json:"devices"`
			Projects      []string         `json:"projects"`
			OmittedTitles map[int64]string `json:"omitted_titles"`
		}{page, devices, projects, titles})
	}
}

// omittedTitles names records a size-limited page left out. The titles travel
// beside the page rather than inside it, so agent-facing references keep their
// existing shape.
func omittedTitles(ctx context.Context, store *gomemory.Writer, page gomemory.Page) (map[int64]string, error) {
	return store.Titles(ctx, page.OmittedIDs)
}
