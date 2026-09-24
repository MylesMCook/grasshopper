package gomcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/MylesMCook/grasshopper/internal/goembed"
	"github.com/MylesMCook/grasshopper/internal/gomemory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testToken = "synthetic-grasshopper-token-0123456789-abcdef"

type failedEmbedder struct{}

type constantEmbedder struct{}

func (constantEmbedder) EmbedDocument(string) ([]float32, error) { return []float32{0, 1}, nil }
func (constantEmbedder) EmbedQuery(string) ([]float32, error)    { return []float32{0, 1}, nil }

func (failedEmbedder) EmbedDocument(string) ([]float32, error) {
	return nil, errors.New("synthetic model failure")
}
func (failedEmbedder) EmbedQuery(string) ([]float32, error) {
	return nil, errors.New("synthetic model failure")
}

type stalledEmbedder struct{ release <-chan struct{} }

func (s stalledEmbedder) EmbedDocument(string) ([]float32, error) {
	<-s.release
	return nil, errors.New("stalled model released")
}
func (s stalledEmbedder) EmbedQuery(string) ([]float32, error) {
	<-s.release
	return nil, errors.New("stalled model released")
}

type bearerTransport struct{ base http.RoundTripper }

func (b bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header.Set("Authorization", "Bearer "+testToken)
	return b.base.RoundTrip(copy)
}

func testServer(t *testing.T, visualizer ...bool) (*httptest.Server, *gomemory.Writer) {
	t.Helper()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	w, err := gomemory.OpenWritableCopy(context.Background(), source, filepath.Join(t.TempDir(), "server-copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(Backend{Store: w, Visualizer: len(visualizer) > 0 && visualizer[0]}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	})
	return server, w
}

func clientSession(t *testing.T, server *httptest.Server, version string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "grasshopper-test", Version: "0.1"}, nil)
	httpClient := &http.Client{Transport: bearerTransport{http.DefaultTransport}, Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true}, &mcp.ClientSessionOptions{ProtocolVersion: version})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	return session
}

func call(t *testing.T, session *mcp.ClientSession, name string, input any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: input})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("%s returned tool error: %+v", name, result)
	}
	if len(result.Content) == 0 || result.StructuredContent == nil {
		t.Fatalf("%s lacked content or structured result: %+v", name, result)
	}
	return result
}

func decodeResult[T any](t *testing.T, result *mcp.CallToolResult) T {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPAuthenticatedFiveToolContract(t *testing.T) {
	server, _ := testServer(t)
	for _, version := range []string{"2026-07-28", "2025-03-26"} {
		t.Run(version, func(t *testing.T) {
			session := clientSession(t, server, version)
			if got := session.InitializeResult().ProtocolVersion; got != version {
				t.Fatalf("negotiated %q, wanted %q", got, version)
			}
			listed, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, 0, len(listed.Tools))
			for _, tool := range listed.Tools {
				names = append(names, tool.Name)
				wantReadOnly := tool.Name == "context" || tool.Name == "get" || tool.Name == "search"
				if tool.Annotations == nil || tool.Annotations.ReadOnlyHint != wantReadOnly {
					t.Fatalf("%s read-only annotation: %+v", tool.Name, tool.Annotations)
				}
			}
			sort.Strings(names)
			if !reflect.DeepEqual(names, []string{"archive", "context", "get", "search", "store"}) {
				t.Fatalf("memory-only tools: %v", names)
			}
			page := decodeResult[gomemory.Page](t, call(t, session, "context", map[string]any{"scope": map[string]any{}, "budget": 16000}))
			if len(page.Records) != 1 || page.Records[0].ID != 1 || page.Records[0].Revision != 2 {
				t.Fatalf("global context: %+v", page)
			}
		})
	}
}

func TestMCPStoreSearchGetArchiveAndConflict(t *testing.T) {
	server, _ := testServer(t)
	session := clientSession(t, server, "2026-07-28")
	project := "id:project-c"
	scope := gomemory.Scope{Project: &project}
	input := gomemory.WriteInput{Scope: scope, Content: "Use pnpm in project C.", Purpose: "decision", Confirmed: true, Provenance: gomemory.Provenance{Harness: "codex", Device: "synthetic-mac", Source: "synthetic MCP test"}, RequestID: "mcp-go-store"}
	stored := decodeResult[storeOutput](t, call(t, session, "store", input))
	if stored.ID < 7 || stored.Revision != 1 || stored.SemanticReady {
		t.Fatalf("store receipt: %+v", stored)
	}
	page := decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": scope, "query": "pnpm"}))
	if len(page.Results.Records) != 1 || page.Results.Records[0].ID != stored.ID {
		t.Fatalf("search: %+v", page)
	}
	record := decodeResult[gomemory.Record](t, call(t, session, "get", map[string]any{"scope": scope, "id": stored.ID}))
	if record.Content != input.Content {
		t.Fatalf("full get: %+v", record)
	}
	archived := decodeResult[gomemory.Receipt](t, call(t, session, "archive", gomemory.ArchiveInput{Scope: scope, ID: stored.ID, ExpectedRevision: 1, Archived: true, RequestID: "mcp-go-archive", Provenance: input.Provenance}))
	if archived.Revision != 2 {
		t.Fatalf("archive: %+v", archived)
	}
	page = decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": scope, "query": "pnpm"}))
	if len(page.Results.Records) != 0 {
		t.Fatalf("archived record in search: %+v", page)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stale, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "archive", Arguments: gomemory.ArchiveInput{Scope: scope, ID: stored.ID, ExpectedRevision: 1, Archived: false, RequestID: "mcp-go-stale", Provenance: input.Provenance}})
	if err != nil {
		t.Fatal(err)
	}
	if !stale.IsError {
		t.Fatalf("stale correction was acknowledged: %+v", stale)
	}
}

func TestMCPRequiresScopeAndSeparatesProjects(t *testing.T) {
	server, _ := testServer(t)
	session := clientSession(t, server, "2026-07-28")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	missing, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"query": "bun"}})
	if err != nil {
		t.Fatal(err)
	}
	if !missing.IsError {
		t.Fatalf("missing explicit scope was accepted: %+v", missing)
	}
	projectA, projectB := "id:project-a", "id:project-b"
	a := decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": map[string]any{"project": projectA}, "query": "bun"}))
	b := decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": map[string]any{"project": projectB}, "query": "bun"}))
	if len(a.Results.Records) != 1 || a.Results.Records[0].ID != 2 || len(b.Results.Records) != 0 {
		t.Fatalf("project scope mixed: A=%+v B=%+v", a, b)
	}
}

func TestMCPRejectsUnauthenticatedAndOversizedRequests(t *testing.T) {
	server, _ := testServer(t)
	for _, url := range []string{server.URL + "/healthz", server.URL + "/mcp"} {
		response, err := http.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s: %d", url, response.StatusCode)
		}
	}
	wrong, err := http.NewRequest(http.MethodGet, server.URL+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	wrong.Header.Set("Authorization", "Bearer "+strings.Repeat("x", len(testToken)))
	wrongResponse, err := http.DefaultClient.Do(wrong)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, wrongResponse.Body)
	wrongResponse.Body.Close()
	if wrongResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong bearer token: %d", wrongResponse.StatusCode)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(strings.Repeat("x", 131073)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: %d", response.StatusCode)
	}
}

func TestMCPRestoreReembedsHistoricalContentWithCurrentModel(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	w, err := gomemory.OpenWritableCopy(context.Background(), source, filepath.Join(t.TempDir(), "restore-model.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	handler, err := NewHandler(Backend{Store: w, Embedder: constantEmbedder{}, Model: "new-model"}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := clientSession(t, server, "2026-07-28")
	project := "id:project-a"
	scope := gomemory.Scope{Project: &project}
	provenance := gomemory.Provenance{Harness: "claude-code", Device: "synthetic-mac", Source: "restore model test"}
	archived := decodeResult[gomemory.Receipt](t, call(t, session, "archive", gomemory.ArchiveInput{Scope: scope, ID: 2, ExpectedRevision: 1, Archived: true, RequestID: "archive-model-test", Provenance: provenance}))
	if archived.Revision != 2 {
		t.Fatalf("archive revision: %+v", archived)
	}
	id, expected, revision := int64(2), int64(2), int64(1)
	restored := decodeResult[storeOutput](t, call(t, session, "store", gomemory.WriteInput{Scope: scope, Content: "", Purpose: "decision", Confirmed: true, Provenance: provenance, RequestID: "restore-model-test", ID: &id, ExpectedRevision: &expected, RestoreRevision: &revision}))
	if restored.Revision != 3 || !restored.SemanticReady {
		t.Fatalf("restore acknowledgement: %+v", restored)
	}
	found := decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": scope, "query": "unmatchedphrase"}))
	if !found.SemanticReady || len(found.Results.Records) != 1 || found.Results.Records[0].ID != 2 {
		t.Fatalf("restored content not searchable through current model: %+v", found)
	}
}

func TestMCPModelFailureDegradesReadWithoutFalseWrite(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	w, err := gomemory.OpenWritableCopy(context.Background(), source, filepath.Join(t.TempDir(), "failed-model.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	handler, err := NewHandler(Backend{Store: w, Embedder: failedEmbedder{}, Model: "synthetic-model"}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := clientSession(t, server, "2026-07-28")
	project := "id:project-a"
	searched := decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": map[string]any{"project": project}, "query": "bun"}))
	if searched.SemanticReady || len(searched.Results.Records) == 0 || searched.Results.Records[0].ID != 2 {
		t.Fatalf("lexical fallback: %+v", searched)
	}
	input := gomemory.WriteInput{Scope: gomemory.Scope{Project: &project}, Content: "A failed model write must not be saved.", Purpose: "lesson", Confirmed: true, Provenance: gomemory.Provenance{Harness: "codex", Device: "synthetic-mac", Source: "failure test"}, RequestID: "model-failed-write"}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "store", Arguments: input})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("failed model write was acknowledged: %+v", result)
	}
	readerPage, err := w.Search(context.Background(), input.Scope, "failed model write", nil, "", 10, 16000)
	if err != nil || len(readerPage.Records) != 0 {
		t.Fatalf("failed write persisted: %+v %v", readerPage, err)
	}
}

func TestMCPStalledInferenceHasBoundedFailure(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	w, err := gomemory.OpenWritableCopy(context.Background(), source, filepath.Join(t.TempDir(), "stalled-model.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	release := make(chan struct{})
	defer close(release)
	handler, err := NewHandler(Backend{Store: w, Embedder: stalledEmbedder{release}, Model: "synthetic-model", InferenceTimeout: 50 * time.Millisecond}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := clientSession(t, server, "2026-07-28")
	project := "id:project-a"
	start := time.Now()
	searched := decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": map[string]any{"project": project}, "query": "bun"}))
	if time.Since(start) > time.Second || searched.SemanticReady || len(searched.Results.Records) == 0 {
		t.Fatalf("stalled read was not bounded: %+v elapsed=%v", searched, time.Since(start))
	}
	input := gomemory.WriteInput{Scope: gomemory.Scope{Project: &project}, Content: "Stalled write must not persist.", Purpose: "lesson", Confirmed: true, Provenance: gomemory.Provenance{Harness: "codex", Device: "synthetic-mac", Source: "timeout test"}, RequestID: "stalled-write"}
	start = time.Now()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "store", Arguments: input})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second || !result.IsError {
		t.Fatalf("stalled write was acknowledged or unbounded: %+v elapsed=%v", result, time.Since(start))
	}
	page, err := w.Search(context.Background(), input.Scope, "stalled write", nil, "", 10, 16000)
	if err != nil || len(page.Records) != 0 {
		t.Fatalf("stalled write persisted: %+v %v", page, err)
	}
}

func TestMCPRealBGEImmediateRecall(t *testing.T) {
	root := os.Getenv("GRASSHOPPER_BGE_TEST_ROOT")
	if root == "" {
		t.Skip("set GRASSHOPPER_BGE_TEST_ROOT for real ONNX MCP test")
	}
	library := filepath.Join(root, "go-probe", "onnxruntime-osx-arm64-1.30.0", "lib", "libonnxruntime.dylib")
	if configured := os.Getenv("GRASSHOPPER_ONNX_RUNTIME_LIBRARY"); configured != "" {
		library = configured
	}
	model := filepath.Join(root, "models", "bge-small-en-v1.5", "onnx", "model.onnx")
	tokenizer := filepath.Join(root, "models", "bge-small-en-v1.5", "tokenizer.json")
	embedder, err := goembed.NewBGE(library, model, tokenizer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := embedder.Close(); err != nil {
			t.Error(err)
		}
	}()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	w, err := gomemory.OpenWritableCopy(context.Background(), source, filepath.Join(t.TempDir(), "mcp-bge-copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	handler, err := NewHandler(Backend{Store: w, Embedder: embedder, Model: goembed.ModelName}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	session := clientSession(t, server, "2026-07-28")
	project := "id:project-c"
	scope := gomemory.Scope{Project: &project}
	content := "Use pnpm for JavaScript packages in project C. " + strings.Repeat("This synthetic decision is scoped to project C. ", 5) + "Only for this compatibility probe."
	stored := decodeResult[storeOutput](t, call(t, session, "store", gomemory.WriteInput{Scope: scope, Content: content, Purpose: "decision", Confirmed: true, Provenance: gomemory.Provenance{Harness: "codex", Device: "synthetic-mac", Source: "real BGE MCP probe"}, RequestID: "mcp-real-bge"}))
	if !stored.SemanticReady || stored.ID < 7 {
		t.Fatalf("model-backed store: %+v", stored)
	}
	query := "Which dependency manager should the third app run?"
	lexical, err := w.Search(context.Background(), scope, query, nil, "", 10, 16000)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range lexical.Records {
		if record.ID == stored.ID {
			t.Fatalf("semantic probe matched lexically: %+v", lexical)
		}
	}
	searched := decodeResult[searchOutput](t, call(t, session, "search", map[string]any{"scope": map[string]any{"project": project}, "query": query}))
	if !searched.SemanticReady || len(searched.Results.Records) == 0 || searched.Results.Records[0].ID != stored.ID {
		t.Fatalf("new BGE record not recalled via MCP: %+v", searched)
	}
	full := decodeResult[gomemory.Record](t, call(t, session, "get", map[string]any{"scope": map[string]any{"project": project}, "id": stored.ID}))
	if full.Content != content || !strings.Contains(full.Content, "Only for this compatibility probe.") {
		t.Fatalf("full qualification missing: %+v", full)
	}
}
