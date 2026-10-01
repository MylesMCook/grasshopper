package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/memory"
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
		if path == "/visualizer/" && (!strings.Contains(string(data), "<title>Grasshopper</title>") || strings.Contains(string(data), "answer-style")) {
			t.Fatal("shell missing its title or embedded a memory")
		}
		if path == "/visualizer/app.js" && (strings.Contains(string(data), "localStorage") || !strings.Contains(string(data), "textContent")) {
			t.Fatal("memory view must render text safely without storing credentials")
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

func TestVisualizerAnnotationStyleHashesAreOptInAndValidated(t *testing.T) {
	server, store := testServer(t, true)
	resp, _ := visualizerRequest(t, http.MethodGet, server.URL+"/visualizer/", "")
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "style-src 'self';") || strings.Contains(csp, "sha256-") {
		t.Fatalf("default style policy changed: %q", csp)
	}
	hash := "sha256-" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	otherHash := "sha256-" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	for _, invalid := range []string{"'unsafe-inline'", "sha256-short", "sha256-" + strings.Repeat("A", 44)} {
		if _, err := NewHandler(Backend{Store: store, Visualizer: true, VisualizerStyleHashes: []string{invalid}}, testToken); err == nil {
			t.Fatalf("accepted invalid style hash %q", invalid)
		}
	}
	if _, err := NewHandler(Backend{Store: store, VisualizerStyleHashes: []string{hash}}, testToken); err == nil {
		t.Fatal("accepted style hash with visualizer disabled")
	}
	handler, err := NewHandler(Backend{Store: store, Visualizer: true, VisualizerStyleHashes: []string{hash, otherHash}}, testToken)
	if err != nil {
		t.Fatal(err)
	}
	withHash := httptest.NewServer(handler)
	defer withHash.Close()
	resp, _ = visualizerRequest(t, http.MethodGet, withHash.URL+"/visualizer/", "")
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "style-src 'self' '"+hash+"' '"+otherHash+"';") || strings.Contains(csp, "unsafe-inline") {
		t.Fatalf("opt-in style policy: %q", csp)
	}
	resp, _ = visualizerRequest(t, http.MethodGet, withHash.URL+"/visualizer/app.js", "")
	if strings.Contains(resp.Header.Get("Content-Security-Policy"), hash) {
		t.Fatal("annotation hash widened non-document response")
	}
}

func TestVisualizerScopedContextUpdatesAndRestart(t *testing.T) {
	server, store := testServer(t, true)
	load := func(address, scope string) memory.Page {
		t.Helper()
		resp, data := visualizerRequest(t, http.MethodPost, address, testToken, scope)
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("visualizer read: status=%d body=%q", resp.StatusCode, data)
		}
		var page memory.Page
		if err := json.Unmarshal(data, &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	base := server.URL + "/visualizer/api/context"
	initial := load(base, "{}")
	if len(initial.Records) != 2 || initial.Records[0].ID != 1 || initial.Records[0].Revision != 2 || initial.Records[1].ID != 4 {
		t.Fatalf("initial cross-platform browse view: %+v", initial)
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
	write := memory.WriteInput{
		Scope: memory.Scope{}, Content: "Synthetic visualizer update after an agent write.",
		Purpose: "preference", Confirmed: true, Provenance: memory.Provenance{Harness: "cursor", Device: "synthetic-mac", Source: "visualizer test"}, RequestID: "visualizer-live-test",
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

func TestVisualizerAnyDeviceBrowsesActiveScopedRecords(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	projectA, projectB, retiredProject, observedProject := "id:project-a", "id:project-b", "id:retired-project", "id:observed-project"
	mac, windows, projectDevice, otherProjectDevice := "synthetic-mac-only", "synthetic-windows-only", "project-a-only", "project-b-only"
	macos, windowsOS := "macos", "windows"
	write := func(scope memory.Scope, content, requestID string, confirmed bool) int64 {
		t.Helper()
		receipt, err := store.Write(ctx, memory.WriteInput{
			Scope: scope, Content: content, Purpose: "lesson", Confirmed: confirmed,
			Provenance: memory.Provenance{Harness: "test", Device: "synthetic", Source: "visualizer device test"},
			RequestID:  requestID,
		}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt.ID
	}
	write(memory.Scope{Device: &mac, Platform: &macos}, "Mac-only synthetic fact", "visualizer-mac", true)
	write(memory.Scope{Device: &windows, Platform: &windowsOS}, "Windows-only synthetic fact", "visualizer-windows", true)
	write(memory.Scope{Project: &projectA, Device: &projectDevice}, "Project A synthetic fact", "visualizer-project-a", true)
	write(memory.Scope{Project: &projectB, Device: &otherProjectDevice}, "Project B synthetic fact", "visualizer-project-b", true)
	write(memory.Scope{Project: &observedProject}, "Unconfirmed project observation", "visualizer-project-observation", false)
	retiredProjectID := write(memory.Scope{Project: &retiredProject}, "Retired project fact", "visualizer-project-retired", true)
	if _, err := store.Archive(ctx, memory.ArchiveInput{
		Scope: memory.Scope{Project: &retiredProject}, ID: retiredProjectID, ExpectedRevision: 1, Archived: true,
		RequestID: "visualizer-project-retired-archive", Provenance: memory.Provenance{Harness: "test", Device: "synthetic", Source: "visualizer device test"},
	}); err != nil {
		t.Fatal(err)
	}
	observed := "observed-only"
	write(memory.Scope{Device: &observed}, "Unconfirmed synthetic observation", "visualizer-observation", false)
	retired := "retired-only"
	retiredID := write(memory.Scope{Device: &retired}, "Archived synthetic fact", "visualizer-retired", true)
	if _, err := store.Archive(ctx, memory.ArchiveInput{
		Scope: memory.Scope{Device: &retired}, ID: retiredID, ExpectedRevision: 1, Archived: true,
		RequestID: "visualizer-retired-archive", Provenance: memory.Provenance{Harness: "test", Device: "synthetic", Source: "visualizer device test"},
	}); err != nil {
		t.Fatal(err)
	}
	load := func(scope string) (memory.Page, []string, []string) {
		t.Helper()
		resp, data := visualizerRequest(t, http.MethodPost, server.URL+"/visualizer/api/context", testToken, scope)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("visualizer scope %s: status=%d body=%s", scope, resp.StatusCode, data)
		}
		var view struct {
			memory.Page
			Devices  []string `json:"devices"`
			Projects []string `json:"projects"`
		}
		if err := json.Unmarshal(data, &view); err != nil {
			t.Fatal(err)
		}
		return view.Page, view.Devices, view.Projects
	}
	contains := func(page memory.Page, content string) bool {
		for _, record := range page.Records {
			if record.Content == content {
				return true
			}
		}
		return false
	}
	global, devices, projects := load(`{}`)
	if !contains(global, "Mac-only synthetic fact") || !contains(global, "Windows-only synthetic fact") || contains(global, "Project A synthetic fact") || !contains(global, "Unconfirmed synthetic observation") || contains(global, "Archived synthetic fact") {
		t.Fatalf("Any device view mixed scope or omitted active device facts: %+v", global)
	}
	agentContext, err := store.Context(ctx, memory.Scope{}, 32768)
	if err != nil || contains(agentContext, "Mac-only synthetic fact") || contains(agentContext, "Windows-only synthetic fact") {
		t.Fatalf("agent context widened with visualizer browsing: page=%+v err=%v", agentContext, err)
	}
	if !slices.Contains(devices, mac) || !slices.Contains(devices, windows) || slices.Contains(devices, projectDevice) || slices.Contains(devices, retired) || !slices.Contains(devices, observed) {
		t.Fatalf("device choices mixed inactive or other-project facts: %v", devices)
	}
	if !slices.Contains(projects, projectA) || !slices.Contains(projects, projectB) || slices.Contains(projects, retiredProject) || !slices.Contains(projects, observedProject) {
		t.Fatalf("project choices included inactive or missed active records: %v", projects)
	}
	project, projectDevices, _ := load(`{"project":"id:project-a"}`)
	if !contains(project, "Project A synthetic fact") || contains(project, "Project B synthetic fact") || !slices.Contains(projectDevices, projectDevice) || slices.Contains(projectDevices, otherProjectDevice) {
		t.Fatalf("project view leaked or omitted scoped device: records=%+v devices=%v", project, projectDevices)
	}
	narrow, _, _ := load(`{"device":"synthetic-mac-only","platform":"macos"}`)
	if !contains(narrow, "Mac-only synthetic fact") || contains(narrow, "Windows-only synthetic fact") {
		t.Fatalf("selected device/platform was not narrow: %+v", narrow)
	}
}

func TestVisualizerListsPreviewsAndDisclosesOverflow(t *testing.T) {
	server, store := testServer(t, true)
	write := func(request, title, content string) {
		t.Helper()
		_, err := store.Write(context.Background(), memory.WriteInput{
			Scope: memory.Scope{}, Title: &title, Content: content, Purpose: "lesson", Confirmed: true,
			Provenance: memory.Provenance{Harness: "codex", Device: "synthetic-mac", Source: "bounded view test"},
			RequestID:  request,
		}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	load := func() (memory.Page, string, int) {
		t.Helper()
		resp, body := visualizerRequest(t, http.MethodPost, server.URL+"/visualizer/api/context", testToken, "{}")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("bounded view status=%d body=%q", resp.StatusCode, body)
		}
		var page struct {
			memory.Page
			Next  string `json:"next"`
			Total int    `json:"total"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		if page.Total > len(page.Records) && page.Next == "" {
			t.Fatal("overflow has no next page")
		}
		return page.Page, page.Next, page.Total
	}
	full := strings.Repeat("alpha ", 4000)
	write("visualizer-bound-large", "Large alpha", full)
	page, _, total := load()
	if total != len(page.Records) {
		t.Fatalf("a large record must not push others out of the list: %+v", page)
	}
	found := false
	for _, record := range page.Records {
		if record.Title == "Large alpha" {
			found = true
			if !record.ContentTruncated || len([]rune(record.Content)) != 240 || !strings.HasPrefix(full, record.Content) {
				t.Fatalf("large record is not a 240-character preview: truncated=%v length=%d", record.ContentTruncated, len([]rune(record.Content)))
			}
		} else if record.ContentTruncated != (len([]rune(record.Content)) == 240) {
			t.Fatalf("truncation flag disagrees with preview length: %+v", record)
		}
	}
	if !found {
		t.Fatal("large record missing from the owner's list")
	}
	for index := 0; index < 120; index++ {
		write("visualizer-bound-many-"+jsonNumber(int64(index)), "Overflow "+jsonNumber(int64(index)), strings.Repeat("beta ", 60))
	}
	page, next, total := load()
	if total <= len(page.Records) || next == "" || len(page.Records) == 0 {
		t.Fatalf("overflow not disclosed: total=%d next=%q records=%d", total, next, len(page.Records))
	}
}
