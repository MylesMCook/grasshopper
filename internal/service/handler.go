// Package service adapts the Go memory store to authenticated MCP.
// It exposes no code indexing or server-filesystem tool.
package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/MylesMCook/grasshopper/internal/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Embedder creates vectors for stored documents and search queries.
type Embedder interface {
	EmbedDocument(string) ([]float32, error)
	EmbedQuery(string) ([]float32, error)
}

// Backend supplies storage, optional inference, and HTTP configuration.
// Store is required; Model is required when Embedder is set.
type Backend struct {
	Store                 *memory.Writer
	Version               string
	Embedder              Embedder
	Model                 string
	InferenceTimeout      time.Duration
	Visualizer            bool
	VisualizerStyleHashes []string
	AllowedProxyHost      string
	// Logger optionally receives secret-free structured operational events.
	Logger *slog.Logger
}

type contextInput struct {
	Scope  memory.Scope `json:"scope"`
	Budget *int         `json:"budget,omitempty"`
}
type getInput struct {
	Scope    memory.Scope `json:"scope"`
	ID       int64        `json:"id"`
	Revision *int64       `json:"revision,omitempty"`
}
type searchInput struct {
	Scope  memory.Scope `json:"scope"`
	Query  string       `json:"query"`
	Limit  *int         `json:"limit,omitempty"`
	Budget *int         `json:"budget,omitempty"`
}
type searchOutput struct {
	Results       memory.Page `json:"results"`
	SemanticReady bool        `json:"semantic_ready"`
}
type storeOutput struct {
	Receipt       memory.Receipt `json:"receipt"`
	ID            int64          `json:"id"`
	Revision      int64          `json:"revision"`
	Deduplicated  bool           `json:"deduplicated"`
	SemanticReady bool           `json:"semantic_ready"`
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
func optional(typeName string) map[string]any {
	return map[string]any{"type": []string{typeName, "null"}}
}
func scopeSchema() map[string]any {
	project := optional("string")
	project["description"] = "Stable project identity, never a folder path: id:<grasshopper.project-id> from local Git config, or git:<normalized origin host/repository path>. Omit if unresolved."
	device := optional("string")
	device["description"] = "Stable device ID from client configuration; omit if unknown."
	platform := optional("string")
	platform["description"] = "Use exactly macos, windows, or linux; never Darwin, win32, or an OS version. Omit if unknown."
	platform["enum"] = []any{"macos", "windows", "linux", nil}
	schema := objectSchema(map[string]any{"project": project, "device": device, "platform": platform, "legacy": map[string]any{"type": "boolean"}})
	schema["description"] = "Read and write only records applicable to this explicit scope."
	return schema
}
func provenanceSchema() map[string]any {
	return objectSchema(map[string]any{"harness": map[string]any{"type": "string"}, "device": map[string]any{"type": "string"}, "source": map[string]any{"type": "string"}}, "harness", "device", "source")
}

// memoryTypeSchema lists the stored kinds the writer accepts. It is separate
// from purpose, which agents set to preference, decision and so on.
func memoryTypeSchema() map[string]any {
	schema := optional("string")
	schema["enum"] = []any{"knowledge", "identity", "episode", "procedure", nil}
	schema["description"] = "Optional; omit it to use knowledge. Not the same as purpose."
	return schema
}
func writeSchema() map[string]any {
	return objectSchema(map[string]any{
		"scope": scopeSchema(), "content": map[string]any{"type": "string", "maxLength": 32768}, "title": optional("string"), "tags": optional("string"),
		"memory_type": memoryTypeSchema(), "purpose": map[string]any{"type": "string", "enum": []string{"preference", "decision", "lesson", "handoff", "observation"}},
		"confirmed": map[string]any{"type": "boolean"}, "provenance": provenanceSchema(), "request_id": map[string]any{"type": "string"},
		"key": optional("string"), "id": optional("integer"), "expected_revision": optional("integer"), "restore_revision": optional("integer"),
	}, "scope", "content", "purpose", "confirmed", "provenance", "request_id")
}

// NewHandler validates the backend and master token, ensures the client-token
// table, and returns the authenticated MCP and optional visualizer handler.
func NewHandler(backend Backend, token string) (http.Handler, error) {
	if backend.Store == nil {
		return nil, errors.New("memory backend required")
	}
	if backend.Embedder != nil && backend.Model == "" {
		return nil, errors.New("embedding model identity required")
	}
	if len(token) < 32 || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return nil, errors.New("token must contain at least 32 non-whitespace characters")
	}
	upgradeCtx, upgradeCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer upgradeCancel()
	if err := backend.Store.EnsureClientTokenSchema(upgradeCtx); err != nil {
		return nil, err
	}
	if backend.Visualizer {
		if err := backend.Store.EnsureOwnerSessionSchema(upgradeCtx); err != nil {
			return nil, err
		}
	}
	if backend.AllowedProxyHost != "" {
		host, port, err := net.SplitHostPort(backend.AllowedProxyHost)
		portNumber, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || portNumber < 1 || portNumber > 65535 || host == "" || strings.ContainsAny(host, "/@?#\\") || strings.IndexFunc(host, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return nil, errors.New("allowed proxy host must be an exact hostname and port")
		}
	}
	if len(backend.VisualizerStyleHashes) > 4 || (!backend.Visualizer && len(backend.VisualizerStyleHashes) > 0) {
		return nil, errors.New("visualizer style hashes require an enabled visualizer and at most four hashes")
	}
	for _, hash := range backend.VisualizerStyleHashes {
		if !strings.HasPrefix(hash, "sha256-") {
			return nil, errors.New("visualizer style hash must be SHA-256")
		}
		digest, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(hash, "sha256-"))
		if err != nil || len(digest) != sha256.Size || base64.StdEncoding.EncodeToString(digest) != strings.TrimPrefix(hash, "sha256-") {
			return nil, errors.New("visualizer style hash must be a canonical SHA-256 digest")
		}
	}
	inferenceTimeout := backend.InferenceTimeout
	if inferenceTimeout <= 0 || inferenceTimeout > 10*time.Second {
		inferenceTimeout = 5 * time.Second
	}
	// A request timeout does not stop an in-flight embedding. Keep the slot
	// occupied until the call returns so timed-out calls cannot overlap.
	inferenceGate := make(chan struct{}, 1)
	runInference := func(ctx context.Context, fn func() ([]float32, error)) ([]float32, error) {
		inferenceCtx, cancel := context.WithTimeout(ctx, inferenceTimeout)
		defer cancel()
		select {
		case inferenceGate <- struct{}{}:
		case <-inferenceCtx.Done():
			return nil, inferenceCtx.Err()
		}
		type result struct {
			vector []float32
			err    error
		}
		completed := make(chan result, 1)
		go func() { defer func() { <-inferenceGate }(); vector, err := fn(); completed <- result{vector, err} }()
		select {
		case outcome := <-completed:
			return outcome.vector, outcome.err
		case <-inferenceCtx.Done():
			return nil, inferenceCtx.Err()
		}
	}
	embedQuery := func(ctx context.Context, query string) ([]float32, error) {
		if backend.Embedder == nil {
			return nil, nil
		}
		return runInference(ctx, func() ([]float32, error) { return backend.Embedder.EmbedQuery(query) })
	}
	// saveMemory embeds the saved text before writing so search finds it at
	// once. The MCP store tool and the owner's memory-view actions share it.
	saveMemory := func(ctx context.Context, in memory.WriteInput) (memory.Receipt, bool, error) {
		var vector []float32
		if backend.Embedder != nil {
			content := in.Content
			if in.RestoreRevision != nil {
				if in.ID == nil {
					return memory.Receipt{}, false, errors.New("restore requires record ID")
				}
				prior, err := backend.Store.Get(ctx, in.Scope, *in.ID, in.RestoreRevision)
				if err != nil {
					return memory.Receipt{}, false, err
				}
				if prior == nil {
					return memory.Receipt{}, false, errors.New("revision_not_found")
				}
				content = prior.Content
			}
			var err error
			vector, err = runInference(ctx, func() ([]float32, error) { return backend.Embedder.EmbedDocument(content) })
			if err != nil {
				return memory.Receipt{}, false, err
			}
		}
		receipt, err := backend.Store.Write(ctx, in, vector, backend.Model)
		return receipt, vector != nil, err
	}
	// moveMemory embeds the moved text, then copies and archives in one
	// transaction. The text comes from the revision the owner saw.
	moveMemory := func(ctx context.Context, in memory.MoveInput, content string) (memory.Receipt, error) {
		var vector []float32
		if backend.Embedder != nil {
			var err error
			vector, err = runInference(ctx, func() ([]float32, error) { return backend.Embedder.EmbedDocument(content) })
			if err != nil {
				return memory.Receipt{}, err
			}
		}
		return backend.Store.Move(ctx, in, vector, backend.Model)
	}
	tokenHash := sha256.Sum256([]byte(token))
	masterBearerValid := func(r *http.Request) bool {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || provided == "" {
			return false
		}
		providedHash := sha256.Sum256([]byte(provided))
		return subtle.ConstantTimeCompare(providedHash[:], tokenHash[:]) == 1
	}
	audit := newAuditLog(backend.Logger)
	sessions := &ownerSessions{audit: audit, store: backend.Store, attempts: make(map[string]loginWindow)}
	ownerValid := func(r *http.Request) bool {
		if masterBearerValid(r) {
			return true
		}
		cookie, err := r.Cookie(visualizerCookieName)
		return err == nil && sessions.valid(r, cookie.Value, tokenHash[:])
	}
	seen := &deviceActivity{last: map[[sha256.Size]byte]time.Time{}}
	bearerValid := func(r *http.Request) bool {
		if masterBearerValid(r) {
			return true
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || len(provided) < 32 || len(provided) > 256 {
			return false
		}
		valid, err := backend.Store.ClientTokenValid(r.Context(), provided)
		if err == nil && valid {
			seen.touch(r.Context(), backend.Store, provided)
		}
		return err == nil && valid
	}
	var pairings *pairingManager
	if backend.Visualizer {
		pairings = newPairingManager(backend.Store)
		pairings.audit = audit
	}
	version := backend.Version
	if version == "" {
		version = "dev"
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "grasshopper", Version: version}, nil)
	falseValue := false
	read := &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &falseValue, OpenWorldHint: &falseValue}
	write := &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &falseValue, OpenWorldHint: &falseValue}
	mcp.AddTool(server, &mcp.Tool{Name: "context", Title: "Load memory context", Description: "Read confirmed preferences, applicable context, and a recent handoff. For project scope, use id:<local grasshopper.project-id> or git:<normalized origin host/path>; never a folder path. Omit project if unresolved.", Annotations: read, InputSchema: objectSchema(map[string]any{"scope": scopeSchema(), "budget": map[string]any{"type": "integer"}}, "scope")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in contextInput) (*mcp.CallToolResult, memory.Page, error) {
			budget := 16384
			if in.Budget != nil {
				budget = *in.Budget
			}
			page, err := backend.Store.Context(ctx, in.Scope, budget)
			return nil, page, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get", Title: "Get complete memory", Description: "Read a full record or earlier revision visible in the current project, device, and platform context. Include the same scope used for context; legacy records need explicit legacy scope.", Annotations: read, InputSchema: objectSchema(map[string]any{"scope": scopeSchema(), "id": map[string]any{"type": "integer"}, "revision": optional("integer")}, "scope", "id")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in getInput) (*mcp.CallToolResult, memory.Record, error) {
			record, err := backend.Store.Get(ctx, in.Scope, in.ID, in.Revision)
			if err != nil {
				return nil, memory.Record{}, err
			}
			if record == nil {
				return nil, memory.Record{}, agentReadError(errors.New("memory_not_found"))
			}
			return nil, *record, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "search", Title: "Search scoped memories", Description: "Find active memories by wording or meaning, limited to the given project, device and platform scope. The limit is 1 to 100.", Annotations: read, InputSchema: objectSchema(map[string]any{"scope": scopeSchema(), "query": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}, "budget": map[string]any{"type": "integer"}}, "scope", "query")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
			limit, budget := 10, 16384
			if in.Limit != nil {
				limit = *in.Limit
			}
			if in.Budget != nil {
				budget = *in.Budget
			}
			vector, err := embedQuery(ctx, in.Query)
			if err != nil {
				vector = nil
			} // Disclose lexical-only recall through semantic_ready.
			page, err := backend.Store.Search(ctx, in.Scope, in.Query, vector, backend.Model, limit, budget)
			return nil, searchOutput{page, vector != nil}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "store", Title: "Save or correct memory", Description: "Use this when saving an explicit preference, accepted decision, verified lesson, or concise handoff with provenance and a request ID. The scope is where the memory applies: {} applies everywhere, project limits it to one project, and device or platform are only for facts about one machine or OS.", Annotations: write, InputSchema: writeSchema()},
		func(ctx context.Context, _ *mcp.CallToolRequest, in memory.WriteInput) (*mcp.CallToolResult, storeOutput, error) {
			receipt, embedded, err := saveMemory(ctx, in)
			return nil, storeOutput{receipt, receipt.ID, receipt.Revision, receipt.Deduplicated, embedded}, agentReadError(err)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "archive", Title: "Archive or restore memory", Description: "Use this when reversibly hiding or restoring a scoped record with an expected revision and request ID.", Annotations: write, InputSchema: objectSchema(map[string]any{"scope": scopeSchema(), "id": map[string]any{"type": "integer"}, "expected_revision": map[string]any{"type": "integer"}, "archived": map[string]any{"type": "boolean"}, "request_id": map[string]any{"type": "string"}, "provenance": provenanceSchema()}, "scope", "id", "expected_revision", "archived", "request_id", "provenance")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in memory.ArchiveInput) (*mcp.CallToolResult, memory.Receipt, error) {
			receipt, err := backend.Store.Archive(ctx, in)
			return nil, receipt, agentReadError(err)
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, MaxRequestBodyBytes: 131072, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The MCP SDK only accepts loopback Hosts here. For the configured HTTPS
		// proxy Host, check Origin above and pass a loopback Host to the SDK;
		// leave all other Hosts to its default check.
		scheme := "http"
		if proxyHostMatches(r.Host, backend.AllowedProxyHost) {
			scheme = "https"
		}
		if origin := r.Header.Get("Origin"); origin != "" && !originsMatch(origin, scheme+"://"+r.Host) {
			http.Error(w, "Origin rejected. Open the local server address or the configured private HTTPS proxy address; check --allowed-proxy-host and restart after configuration changes.", http.StatusForbidden)
			return
		}
		if scheme == "https" {
			copy := r.Clone(r.Context())
			copy.Host = "127.0.0.1"
			mcpHandler.ServeHTTP(w, copy)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	var ownerReads map[string]http.HandlerFunc
	if backend.Visualizer {
		// A model is reported only when something uses it to embed queries.
		searchModel := ""
		if backend.Embedder != nil {
			searchModel = backend.Model
		}
		mux.HandleFunc("/visualizer/api/context", visualizerContext(backend.Store, version, searchModel))
		ownerReads = map[string]http.HandlerFunc{
			"/visualizer/api/search":             visualizerSearch(backend, embedQuery),
			"/visualizer/api/record":             visualizerRecord(backend.Store),
			"/visualizer/api/update":             visualizerUpdate(backend.Store, saveMemory, audit),
			"/visualizer/api/archive":            visualizerArchive(backend.Store, audit),
			"/visualizer/api/move":               visualizerMove(backend.Store, moveMemory, audit),
			"/visualizer/api/startup":            visualizerStartup(backend.Store),
			"/visualizer/api/export":             visualizerExport(backend.Store),
			"/visualizer/api/session/revoke-all": sessions.revokeAll,
		}
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	audit.event("startup", nil, auditFields{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Browser owner APIs and MCP accept only local access or the explicitly
		// configured private proxy. Never reflect the supplied Host in errors.
		if r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/visualizer/api/") {
			host := r.Host
			if parsed, _, err := net.SplitHostPort(host); err == nil {
				host = parsed
			}
			ip := net.ParseIP(host)
			local := host == "localhost" || (ip != nil && ip.IsLoopback())
			if !local && !proxyHostMatches(r.Host, backend.AllowedProxyHost) {
				http.Error(w, "Host rejected. Use the local server address or configure --allowed-proxy-host with the exact private HTTPS proxy hostname and port, then restart the server.", http.StatusForbidden)
				return
			}
		}

		if r.URL.Path == "/" || (r.URL.Path == "/mcp" && r.Method == http.MethodGet && acceptsBrowserHTML(r.Header.Get("Accept"))) {
			serverNavigation(w, r, backend.Visualizer)
			return
		}
		if r.URL.Path == "/favicon.ico" {
			http.NotFound(w, r)
			return
		}
		// Connection identity is bearer-only and reports only this credential.
		// It neither grants owner controls nor changes the five-tool MCP surface.
		if r.URL.Path == "/connection" {
			w.Header().Set("Cache-Control", "no-store")
			role, device := "owner", ""
			if !masterBearerValid(r) {
				provided, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
				if !ok || len(provided) < 32 || len(provided) > 256 {
					http.Error(w, "authentication required", 401)
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				defer cancel()
				var err error
				device, err = backend.Store.ClientTokenDevice(ctx, provided)
				if err != nil {
					http.Error(w, "connection unavailable", 503)
					return
				}
				if device == "" {
					http.Error(w, "authentication required", 401)
					return
				}
				role = "device"
			}
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				http.Error(w, "method not allowed", 405)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"role": role, "device": device, "version": version})
			return
		}
		if backend.Visualizer && visualizerAsset(w, r, backend.VisualizerStyleHashes) {
			return
		}
		if (r.URL.Path == "/visualizer" || strings.HasPrefix(r.URL.Path, "/visualizer/")) && !strings.HasPrefix(r.URL.Path, "/visualizer/api/") {
			http.NotFound(w, r)
			return
		}
		if pairings != nil {
			w.Header().Set("Cache-Control", "no-store")
			switch r.URL.Path {
			case "/pair/start":
				pairings.start(w, r, visualizerOrigin(r, backend.AllowedProxyHost))
				return
			case "/pair/poll":
				pairings.poll(w, r, visualizerOrigin(r, backend.AllowedProxyHost))
				return
			case "/visualizer/api/pairings", "/visualizer/api/devices":
				// The master bearer or owner browser session manages these
				// routes. Paired device bearers may use MCP, but not this API.
				if !ownerValid(r) {
					http.Error(w, "authentication required", http.StatusUnauthorized)
					return
				}
				origin := visualizerOrigin(r, backend.AllowedProxyHost)
				if (r.Method != http.MethodGet && !originsMatch(r.Header.Get("Origin"), origin)) || (r.Header.Get("Origin") != "" && !originsMatch(r.Header.Get("Origin"), origin)) {
					http.Error(w, "Origin rejected. Open the local server address or the configured private HTTPS proxy address; check --allowed-proxy-host and restart after configuration changes.", http.StatusForbidden)
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				if r.URL.Path == "/visualizer/api/pairings" {
					pairings.adminPairings(w, r.WithContext(ctx))
				} else {
					pairings.adminDevices(w, r.WithContext(ctx))
				}
				return
			}
		}
		if backend.Visualizer && r.URL.Path == "/visualizer/api/session" {
			sessions.serve(w, r, tokenHash[:], masterBearerValid(r), backend.AllowedProxyHost)
			return
		}
		if ownerRead, ok := ownerReads[r.URL.Path]; ok {
			// Owner inspection accepts only the master bearer or owner session.
			// Require the exact Origin for both; paired agents retain the MCP API.
			w.Header().Set("Cache-Control", "no-store")
			if !ownerValid(r) {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			if !originsMatch(r.Header.Get("Origin"), visualizerOrigin(r, backend.AllowedProxyHost)) {
				http.Error(w, "Origin rejected. Open the local server address or the configured private HTTPS proxy address; check --allowed-proxy-host and restart after configuration changes.", http.StatusForbidden)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			ownerRead(w, r.WithContext(ctx))
			return
		}
		// Context also accepts an owner browser session after an exact Origin
		// check. MCP continues to require a bearer token.
		authorized := bearerValid(r)
		if !authorized && backend.Visualizer && r.URL.Path == "/visualizer/api/context" {
			if cookie, err := r.Cookie(visualizerCookieName); err == nil && sessions.valid(r, cookie.Value, tokenHash[:]) {
				if !originsMatch(r.Header.Get("Origin"), visualizerOrigin(r, backend.AllowedProxyHost)) {
					http.Error(w, "Origin rejected. Open the local server address or the configured private HTTPS proxy address; check --allowed-proxy-host and restart after configuration changes.", http.StatusForbidden)
					return
				}
				authorized = true
			}
		}
		if !authorized {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	return audit.wrap(handler), nil
}

// agentReadError keeps missing and out-of-scope records indistinguishable while
// explaining how an agent can correct its request without widening scope.
func agentReadError(err error) error {
	if err != nil && err.Error() == "memory_not_found" {
		return errors.New("memory_not_found: not found in this project, device and platform scope; check the scope used for context")
	}
	return err
}

// deviceActivity throttles last-seen writes to one per device token a minute.
// A failed write is ignored: last seen is informational and must never block
// an authenticated request.
type deviceActivity struct {
	mu   sync.Mutex
	last map[[sha256.Size]byte]time.Time
}

func (d *deviceActivity) touch(ctx context.Context, store *memory.Writer, token string) {
	key, now := sha256.Sum256([]byte(token)), time.Now()
	d.mu.Lock()
	if now.Sub(d.last[key]) < time.Minute {
		d.mu.Unlock()
		return
	}
	d.last[key] = now
	d.mu.Unlock()
	// The write runs after the request is answered, bounded to one second.
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = store.TouchClientToken(ctx, token, now)
	}()
}
