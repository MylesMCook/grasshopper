package gomcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func visualizerRequest(t *testing.T, method, address, bearer string, body ...string) (*http.Response, []byte) {
	t.Helper()
	var input io.Reader
	if len(body) > 0 {
		input = strings.NewReader(body[0])
	}
	req, err := http.NewRequest(method, address, input)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, data
}

func TestVisualizerShellDoesNotContainMemoryAndAPIRequiresBearer(t *testing.T) {
	disabled, _ := testServer(t)
	resp, _ := visualizerRequest(t, http.MethodGet, disabled.URL+"/visualizer/", testToken)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled visualizer asset status: %d", resp.StatusCode)
	}
	resp, _ = visualizerRequest(t, http.MethodPost, disabled.URL+"/visualizer/api/context", testToken, "{}")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled visualizer API status: %d", resp.StatusCode)
	}
	server, _ := testServer(t, true)
	for _, path := range []string{"/visualizer/", "/visualizer/app.js", "/visualizer/style.css", "/visualizer/newsreader-latin.woff2"} {
		resp, data := visualizerRequest(t, http.MethodGet, server.URL+path, "")
		if resp.StatusCode != http.StatusOK || len(data) == 0 || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("public shell asset %s: status=%d bytes=%d", path, resp.StatusCode, len(data))
		}
		if path == "/visualizer/" && (!strings.Contains(string(data), "See what your agents remember") || strings.Contains(string(data), "answer-style")) {
			t.Fatal("shell missing its title or embedded a memory")
		}
		if strings.Contains(path, ".js") && (!strings.Contains(string(data), "textContent") || strings.Contains(string(data), "localStorage")) {
			t.Fatal("client does not render text safely or stores token")
		}
	}
	for _, bearer := range []string{"", "wrong-token"} {
		resp, data := visualizerRequest(t, http.MethodPost, server.URL+"/visualizer/api/context", bearer, "{}")
		if resp.StatusCode != http.StatusUnauthorized || strings.Contains(string(data), "answer-style") {
			t.Fatalf("unauthenticated read leaked memory: status=%d body=%q", resp.StatusCode, data)
		}
	}
	resp, _ = visualizerRequest(t, http.MethodPost, server.URL+"/visualizer/app.js", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("asset accepted POST: %d", resp.StatusCode)
	}
}

func TestVisualizerScopedContextUpdatesAndRestart(t *testing.T) {
	server, store := testServer(t, true)
	load := func(address, scope string) gomemory.Page {
		t.Helper()
		resp, data := visualizerRequest(t, http.MethodPost, address, testToken, scope)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("visualizer read: status=%d body=%q", resp.StatusCode, data)
		}
		var page gomemory.Page
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	base := server.URL + "/visualizer/api/context"
	initial := load(base, "{}")
	if len(initial.Records) != 1 || initial.Records[0].ID != 1 || initial.Records[0].Revision != 2 {
		t.Fatalf("initial global context: %+v", initial)
	}
	projectA := load(base, `{"project":"id:project-a"}`)
	projectB := load(base, `{"project":"id:project-b"}`)
	if len(projectA.Records) < 2 || len(projectB.Records) < 2 {
		t.Fatalf("project records missing: A=%+v B=%+v", projectA, projectB)
	}
	for _, record := range projectA.Records {
		if record.Scope.Project != nil && *record.Scope.Project == "id:project-b" {
			t.Fatal("project B leaked into project A")
		}
	}
	for _, invalid := range []string{`{"legacy":true}`, `{"platform":"solaris"}`, `{"unknown":"x"}`, `{"project":"id:x"} {}`} {
		resp, _ := visualizerRequest(t, http.MethodPost, base, testToken, invalid)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid scope %q accepted: %d", invalid, resp.StatusCode)
		}
	}
	write := gomemory.WriteInput{
		Scope: gomemory.Scope{}, Content: "Synthetic visualizer update after an agent write.",
		Purpose: "preference", Confirmed: true, Provenance: gomemory.Provenance{Harness: "cursor", Device: "synthetic-mac", Source: "visualizer test"}, RequestID: "visualizer-live-test",
	}
	receipt, err := store.Write(context.Background(), write, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	updated := load(base, "{}")
	found := false
	for _, record := range updated.Records {
		if record.ID == receipt.ID && record.Revision == receipt.Revision && record.Provenance.Harness == "cursor" && record.Content == write.Content {
			found = true
		}
	}
	if !found {
		t.Fatalf("new record missing without restart: %+v", updated)
	}
	server.Close()
	newHandler, err := NewHandler(Backend{Store: store, Visualizer: true}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	restarted := httptest.NewServer(newHandler)
	defer restarted.Close()
	if page := load(restarted.URL+"/visualizer/api/context", "{}"); len(page.Records) != len(updated.Records) {
		t.Fatalf("restart changed context: before=%d after=%d", len(updated.Records), len(page.Records))
	}
}

func TestVisualizerDisclosesBoundedOmissions(t *testing.T) {
	server, store := testServer(t, true)
	for index, content := range []string{strings.Repeat("alpha ", 4000), strings.Repeat("beta ", 4000)} {
		_, err := store.Write(context.Background(), gomemory.WriteInput{
			Scope: gomemory.Scope{}, Content: content, Purpose: "lesson", Confirmed: true,
			Provenance: gomemory.Provenance{Harness: "codex", Device: "synthetic-mac", Source: "bounded view test"},
			RequestID:  "visualizer-bound-" + string(rune('a'+index)),
		}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	resp, body := visualizerRequest(t, http.MethodPost, server.URL+"/visualizer/api/context", testToken, "{}")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bounded view status=%d body=%q", resp.StatusCode, body)
	}
	var page gomemory.Page
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if page.Omitted == 0 || len(page.OmittedIDs) == 0 || len(page.Records) == 0 {
		t.Fatalf("omissions not disclosed: %+v", page)
	}
	for _, record := range page.Records {
		if strings.HasPrefix(record.Content, "alpha ") && record.Content != strings.Repeat("alpha ", 4000) {
			t.Fatal("returned content was truncated")
		}
	}
}
