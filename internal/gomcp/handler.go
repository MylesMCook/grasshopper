// Package gomcp adapts the Go memory store to authenticated MCP.
// It exposes no code indexing or server-filesystem tool.
package gomcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Embedder interface {
	EmbedDocument(string) ([]float32, error)
	EmbedQuery(string) ([]float32, error)
}

type Backend struct {
	Store                 *gomemory.Writer
	Version               string
	Embedder              Embedder
	Model                 string
	InferenceTimeout      time.Duration
	Visualizer            bool
	VisualizerStyleHashes []string
	AllowedProxyHost      string
}

type contextInput struct {
	Scope  gomemory.Scope `json:"scope"`
	Budget *int           `json:"budget,omitempty"`
}
type getInput struct {
	Scope    gomemory.Scope `json:"scope"`
	ID       int64          `json:"id"`
	Revision *int64         `json:"revision,omitempty"`
}
type searchInput struct {
	Scope  gomemory.Scope `json:"scope"`
	Query  string         `json:"query"`
	Limit  *int           `json:"limit,omitempty"`
	Budget *int           `json:"budget,omitempty"`
}
type searchOutput struct {
	Results       gomemory.Page `json:"results"`
	SemanticReady bool          `json:"semantic_ready"`
}
type storeOutput struct {
	Receipt       gomemory.Receipt `json:"receipt"`
	ID            int64            `json:"id"`
	Revision      int64            `json:"revision"`
	Deduplicated  bool             `json:"deduplicated"`
	SemanticReady bool             `json:"semantic_ready"`
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
func writeSchema() map[string]any {
	return objectSchema(map[string]any{
		"scope": scopeSchema(), "content": map[string]any{"type": "string", "maxLength": 32768}, "title": optional("string"), "tags": optional("string"),
		"memory_type": optional("string"), "purpose": map[string]any{"type": "string", "enum": []string{"preference", "decision", "lesson", "handoff", "observation"}},
		"confirmed": map[string]any{"type": "boolean"}, "provenance": provenanceSchema(), "request_id": map[string]any{"type": "string"},
		"key": optional("string"), "id": optional("integer"), "expected_revision": optional("integer"), "restore_revision": optional("integer"),
	}, "scope", "content", "purpose", "confirmed", "provenance", "request_id")
}

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
	tokenHash := sha256.Sum256([]byte(token))
	masterBearerValid := func(r *http.Request) bool {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || provided == "" {
			return false
		}
		providedHash := sha256.Sum256([]byte(provided))
		return subtle.ConstantTimeCompare(providedHash[:], tokenHash[:]) == 1
	}
	bearerValid := func(r *http.Request) bool {
		if masterBearerValid(r) {
			return true
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || len(provided) < 32 || len(provided) > 256 {
			return false
		}
		valid, err := backend.Store.ClientTokenValid(r.Context(), provided)
		return err == nil && valid
	}
	var pairings *pairingManager
	if backend.Visualizer {
		pairings = newPairingManager(backend.Store)
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
		func(ctx context.Context, _ *mcp.CallToolRequest, in contextInput) (*mcp.CallToolResult, gomemory.Page, error) {
			budget := 16384
			if in.Budget != nil {
				budget = *in.Budget
			}
			page, err := backend.Store.Context(ctx, in.Scope, budget)
			return nil, page, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get", Title: "Get complete memory", Description: "Read a full record or earlier revision visible in the current project, device, and platform context. Include the same scope used for context; legacy records need explicit legacy scope.", Annotations: read, InputSchema: objectSchema(map[string]any{"scope": scopeSchema(), "id": map[string]any{"type": "integer"}, "revision": optional("integer")}, "scope", "id")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in getInput) (*mcp.CallToolResult, gomemory.Record, error) {
			record, err := backend.Store.Get(ctx, in.Scope, in.ID, in.Revision)
			if err != nil {
				return nil, gomemory.Record{}, err
			}
			if record == nil {
				return nil, gomemory.Record{}, errors.New("memory_not_found")
			}
			return nil, *record, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "search", Title: "Search scoped memories", Description: "Use this when locating active memories by wording or meaning within explicit scope.", Annotations: read, InputSchema: objectSchema(map[string]any{"scope": scopeSchema(), "query": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}, "budget": map[string]any{"type": "integer"}}, "scope", "query")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, searchOutput, error) {
			limit, budget := 10, 16384
			if in.Limit != nil {
				limit = *in.Limit
			}
			if in.Budget != nil {
				budget = *in.Budget
			}
			var vector []float32
			if backend.Embedder != nil {
				var err error
				vector, err = runInference(ctx, func() ([]float32, error) { return backend.Embedder.EmbedQuery(in.Query) })
				if err != nil {
					vector = nil // Disclose lexical-only recall through semantic_ready.
				}
			}
			page, err := backend.Store.Search(ctx, in.Scope, in.Query, vector, backend.Model, limit, budget)
			return nil, searchOutput{page, vector != nil}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "store", Title: "Save or correct memory", Description: "Use this when saving an explicit preference, accepted decision, verified lesson, or concise handoff with provenance and a request ID.", Annotations: write, InputSchema: writeSchema()},
		func(ctx context.Context, _ *mcp.CallToolRequest, in gomemory.WriteInput) (*mcp.CallToolResult, storeOutput, error) {
			var vector []float32
			if backend.Embedder != nil {
				content := in.Content
				if in.RestoreRevision != nil {
					if in.ID == nil {
						return nil, storeOutput{}, errors.New("restore requires record ID")
					}
					prior, err := backend.Store.Get(ctx, in.Scope, *in.ID, in.RestoreRevision)
					if err != nil {
						return nil, storeOutput{}, err
					}
					if prior == nil {
						return nil, storeOutput{}, errors.New("revision_not_found")
					}
					content = prior.Content
				}
				var err error
				vector, err = runInference(ctx, func() ([]float32, error) { return backend.Embedder.EmbedDocument(content) })
				if err != nil {
					return nil, storeOutput{}, err
				}
			}
			receipt, err := backend.Store.Write(ctx, in, vector, backend.Model)
			return nil, storeOutput{receipt, receipt.ID, receipt.Revision, receipt.Deduplicated, vector != nil}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "archive", Title: "Archive or restore memory", Description: "Use this when reversibly hiding or restoring a scoped record with an expected revision and request ID.", Annotations: write, InputSchema: objectSchema(map[string]any{"scope": scopeSchema(), "id": map[string]any{"type": "integer"}, "expected_revision": map[string]any{"type": "integer"}, "archived": map[string]any{"type": "boolean"}, "request_id": map[string]any{"type": "string"}, "provenance": provenanceSchema()}, "scope", "id", "expected_revision", "archived", "request_id", "provenance")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in gomemory.ArchiveInput) (*mcp.CallToolResult, gomemory.Receipt, error) {
			receipt, err := backend.Store.Archive(ctx, in)
			return nil, receipt, err
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, MaxRequestBodyBytes: 131072, PropagateRequestCancellation: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The SDK permits only loopback Host headers on a loopback listener.
		// Tailscale Serve preserves the HTTPS Host, so admit one configured
		// proxy Host and retain the SDK's default check for every other Host.
		scheme := "http"
		if backend.AllowedProxyHost != "" && r.Host == backend.AllowedProxyHost {
			scheme = "https"
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != scheme+"://"+r.Host {
			http.Error(w, "invalid Origin header", http.StatusForbidden)
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
	if backend.Visualizer {
		mux.HandleFunc("/visualizer/api/context", visualizerContext(backend.Store))
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if backend.Visualizer && visualizerAsset(w, r, backend.VisualizerStyleHashes) {
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
				admin := masterBearerValid(r)
				if cookie, err := r.Cookie(visualizerCookieName); err == nil && validVisualizerSession(cookie.Value, tokenHash[:], time.Now()) {
					admin = true
				}
				if !admin {
					http.Error(w, "authentication required", http.StatusUnauthorized)
					return
				}
				origin := visualizerOrigin(r, backend.AllowedProxyHost)
				if (r.Method != http.MethodGet && r.Header.Get("Origin") != origin) || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
					http.Error(w, "invalid Origin header", http.StatusForbidden)
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
			visualizerSession(w, r, tokenHash[:], masterBearerValid(r), backend.AllowedProxyHost)
			return
		}
		authorized := bearerValid(r)
		if !authorized && backend.Visualizer && r.URL.Path == "/visualizer/api/context" {
			if cookie, err := r.Cookie(visualizerCookieName); err == nil && validVisualizerSession(cookie.Value, tokenHash[:], time.Now()) {
				if r.Header.Get("Origin") != visualizerOrigin(r, backend.AllowedProxyHost) {
					http.Error(w, "invalid Origin header", http.StatusForbidden)
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
	}), nil
}
