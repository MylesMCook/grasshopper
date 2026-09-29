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

func decodeVisualizerRead(w http.ResponseWriter, r *http.Request, input any, limit ...int64) bool {
	maximum := int64(32768)
	if len(limit) > 0 {
		maximum = limit[0]
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maximum))
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

// serverInfo is what the owner's view reports about the running service.
type serverInfo struct {
	Version  string `json:"version"`
	Model    string `json:"model"`
	Active   int    `json:"memories"`
	Archived int    `json:"archived"`
}

func visualizerContext(store *gomemory.Writer, version, model string) http.HandlerFunc {
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
		var review, archived int
		if err == nil {
			devices, err = visualizerDevices(r.Context(), store, devices)
		}
		if err == nil {
			titles, err = omittedTitles(r.Context(), store, page)
		}
		if err == nil {
			review, archived, err = store.BrowseCounts(r.Context(), scope)
		}
		info := serverInfo{Version: version, Model: model}
		if err == nil {
			info.Active, info.Archived, err = store.Totals(r.Context())
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
			Review        int              `json:"review_count"`
			Archived      int              `json:"archived_count"`
			Server        serverInfo       `json:"server"`
		}{page, devices, projects, titles, review, archived, info})
	}
}

// omittedTitles names records a size-limited page left out. The titles travel
// beside the page rather than inside it, so agent-facing references keep their
// existing shape.
func omittedTitles(ctx context.Context, store *gomemory.Writer, page gomemory.Page) (map[int64]string, error) {
	return store.Titles(ctx, page.OmittedIDs)
}

// The owner's actions are attributed to the memory view, not to an agent.
var ownerProvenance = gomemory.Provenance{Harness: "memory-view", Device: "owner browser", Source: "Owner action in the memory view"}

type ownerSave func(context.Context, gomemory.WriteInput) (gomemory.Receipt, bool, error)

// visualizerUpdate saves the owner's text as a new revision and confirms it.
// The exact stored scope, tags, type and key come from the record itself, so
// a client cannot redirect a write to another scope. All editable fields are
// required, which keeps a retry with the same request ID byte-identical.
func visualizerUpdate(store *gomemory.Writer, save ownerSave) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID               int64  `json:"id"`
			ExpectedRevision int64  `json:"expected_revision"`
			Title            string `json:"title"`
			Content          string `json:"content"`
			Purpose          string `json:"purpose"`
			RequestID        string `json:"request_id"`
		}
		if !decodeVisualizerRead(w, r, &input, 65536) {
			return
		}
		if input.ID < 1 || input.ExpectedRevision < 1 {
			http.Error(w, "invalid memory update", http.StatusBadRequest)
			return
		}
		current, err := store.RecordByID(r.Context(), input.ID, nil)
		if err != nil {
			http.Error(w, "memory unavailable", http.StatusServiceUnavailable)
			return
		}
		if current == nil {
			http.Error(w, "memory not found", http.StatusNotFound)
			return
		}
		if current.Archived {
			ownerConflict(w, r, store, input.ID, "archived")
			return
		}
		var title *string
		if strings.TrimSpace(input.Title) != "" {
			title = &input.Title
		}
		receipt, _, err := save(r.Context(), gomemory.WriteInput{
			Scope: current.Scope, Content: input.Content, Title: title, Tags: &current.Tags, MemoryType: &current.MemoryType,
			Purpose: input.Purpose, Confirmed: true, Provenance: ownerProvenance, RequestID: input.RequestID,
			Key: current.Key, ID: &input.ID, ExpectedRevision: &input.ExpectedRevision,
		})
		ownerWriteResult(w, r, store, input.ID, receipt, err)
	}
}

// visualizerArchive hides or restores a memory at an expected revision.
func visualizerArchive(store *gomemory.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ID               int64  `json:"id"`
			ExpectedRevision int64  `json:"expected_revision"`
			Archived         bool   `json:"archived"`
			RequestID        string `json:"request_id"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if input.ID < 1 || input.ExpectedRevision < 1 {
			http.Error(w, "invalid archive request", http.StatusBadRequest)
			return
		}
		current, err := store.RecordByID(r.Context(), input.ID, nil)
		if err != nil {
			http.Error(w, "memory unavailable", http.StatusServiceUnavailable)
			return
		}
		if current == nil {
			http.Error(w, "memory not found", http.StatusNotFound)
			return
		}
		receipt, err := store.Archive(r.Context(), gomemory.ArchiveInput{Scope: current.Scope, ID: input.ID, ExpectedRevision: input.ExpectedRevision, Archived: input.Archived, RequestID: input.RequestID, Provenance: ownerProvenance})
		ownerWriteResult(w, r, store, input.ID, receipt, err)
	}
}

func ownerWriteResult(w http.ResponseWriter, r *http.Request, store *gomemory.Writer, id int64, receipt gomemory.Receipt, err error) {
	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(receipt)
	case strings.HasPrefix(err.Error(), "revision_conflict"):
		ownerConflict(w, r, store, id, "revision_conflict")
	case strings.HasPrefix(err.Error(), "idempotency_conflict"):
		ownerConflict(w, r, store, id, "idempotency_conflict")
	case strings.HasPrefix(err.Error(), "memory_not_found"):
		http.Error(w, "memory not found", http.StatusNotFound)
	case strings.HasPrefix(err.Error(), "content must be"), strings.HasPrefix(err.Error(), "invalid purpose"),
		strings.HasPrefix(err.Error(), "metadata too large"), strings.HasPrefix(err.Error(), "request_id must"):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "memory not saved", http.StatusServiceUnavailable)
	}
}

// ownerConflict rejects a stale or contradictory change without writing and
// returns the record as it now stands, so the owner can reconcile.
func ownerConflict(w http.ResponseWriter, r *http.Request, store *gomemory.Writer, id int64, reason string) {
	current, err := store.RecordByID(r.Context(), id, nil)
	if err != nil {
		http.Error(w, "memory unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusConflict)
	_ = json.NewEncoder(w).Encode(struct {
		Error   string           `json:"error"`
		Current *gomemory.Record `json:"current"`
	}{reason, current})
}

// visualizerStartup previews the startup context the service would send an
// agent for one exact scope and one of the two budgets its clients use.
func visualizerStartup(store *gomemory.Writer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Scope  gomemory.Scope `json:"scope"`
			Budget int            `json:"budget"`
		}
		if !decodeVisualizerRead(w, r, &input) {
			return
		}
		if _, err := input.Scope.Key(); err != nil || input.Scope.Legacy || (input.Budget != 12000 && input.Budget != 3000) {
			http.Error(w, "invalid startup preview", http.StatusBadRequest)
			return
		}
		preview, err := store.StartupPreview(r.Context(), input.Scope, input.Budget)
		if err != nil {
			http.Error(w, "startup preview unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(preview)
	}
}
