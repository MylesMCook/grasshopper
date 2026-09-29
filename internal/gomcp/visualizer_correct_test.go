package gomcp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func agentSave(t *testing.T, store *gomemory.Writer, request, title, content, purpose string, confirmed bool) gomemory.Receipt {
	t.Helper()
	project := "id:review"
	receipt, err := store.Write(context.Background(), gomemory.WriteInput{
		Scope: gomemory.Scope{Project: &project}, Title: &title, Content: content, Purpose: purpose, Confirmed: confirmed,
		Provenance: gomemory.Provenance{Harness: "codex-desktop", Device: "mac-mini", Source: "review regression"}, RequestID: request,
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func ownerJSON(t *testing.T, server *httptest.Server, path string, body any) (int, map[string]json.RawMessage) {
	t.Helper()
	raw, _ := json.Marshal(body)
	status, data := ownerRead(t, server, path, string(raw))
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(data, &out)
	return status, out
}

func listed(t *testing.T, server *httptest.Server, scope string) (map[string]bool, map[string]int) {
	t.Helper()
	status, data := ownerRead(t, server, "/visualizer/api/context", scope)
	if status != http.StatusOK {
		t.Fatalf("list %s status=%d body=%s", scope, status, data)
	}
	var page struct {
		gomemory.Page
		Review   int `json:"review_count"`
		Archived int `json:"archived_count"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	titles := map[string]bool{}
	for _, record := range page.Records {
		titles[record.Title] = true
	}
	return titles, map[string]int{"review": page.Review, "archived": page.Archived}
}

func TestOwnerReviewsCorrectsAndConfirmsAMemory(t *testing.T) {
	server, store := testServer(t, true)
	observation := agentSave(t, store, "review-observation", "Weekly summary", "The owner might like a weekly summary.", "observation", false)
	agentSave(t, store, "review-handoff", "Latest handoff", "Next action: verify recovery.", "handoff", false)
	agentSave(t, store, "review-confirmed", "Settled decision", "Backups are encrypted.", "decision", true)

	titles, counts := listed(t, server, `{"all_projects":true,"view":"review"}`)
	if !titles["Weekly summary"] || titles["Latest handoff"] || titles["Settled decision"] || counts["review"] != 1 {
		t.Fatalf("review queue must hold only unconfirmed non-handoff memories: %v %v", titles, counts)
	}
	agentBefore, err := store.Context(context.Background(), gomemory.Scope{Project: observation2project()}, 16384)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range agentBefore.Records {
		if record.ID == observation.ID {
			t.Fatal("an unconfirmed observation must not load at agent startup")
		}
	}

	status, body := ownerJSON(t, server, "/visualizer/api/update", map[string]any{
		"id": observation.ID, "expected_revision": observation.Revision, "title": "Weekly summary",
		"content": "Send the owner a weekly summary on Fridays.", "purpose": "preference", "request_id": "owner-edit-1",
	})
	if status != http.StatusOK {
		t.Fatalf("update status=%d body=%s", status, body)
	}
	var receipt gomemory.Receipt
	raw, _ := json.Marshal(body)
	_ = json.Unmarshal(raw, &receipt)
	if receipt.ID != observation.ID || receipt.Revision != 2 {
		t.Fatalf("unexpected receipt %+v", receipt)
	}
	current, err := store.RecordByID(context.Background(), observation.ID, nil)
	if err != nil || current == nil || current.Content != "Send the owner a weekly summary on Fridays." || !current.Confirmed || current.Purpose != "preference" || current.Provenance.Harness != "memory-view" {
		t.Fatalf("owner edit did not confirm and attribute the memory: %+v err=%v", current, err)
	}
	one := int64(1)
	earlier, err := store.RecordByID(context.Background(), observation.ID, &one)
	if err != nil || earlier == nil || earlier.Content != "The owner might like a weekly summary." || earlier.Confirmed || earlier.Provenance.Harness != "codex-desktop" {
		t.Fatalf("earlier revision was not preserved: %+v err=%v", earlier, err)
	}
	if _, counts := listed(t, server, `{"all_projects":true}`); counts["review"] != 0 {
		t.Fatalf("confirmed memory still awaits review: %v", counts)
	}
	agentAfter, err := store.Context(context.Background(), gomemory.Scope{Project: observation2project()}, 16384)
	if err != nil {
		t.Fatal(err)
	}
	loaded := false
	for _, record := range agentAfter.Records {
		loaded = loaded || (record.ID == observation.ID && record.Content == "Send the owner a weekly summary on Fridays.")
	}
	if !loaded {
		t.Fatalf("agents do not read the corrected text: %+v", agentAfter)
	}

	// A retry with the same request and payload replays; a different payload cannot reuse it.
	status, _ = ownerJSON(t, server, "/visualizer/api/update", map[string]any{
		"id": observation.ID, "expected_revision": observation.Revision, "title": "Weekly summary",
		"content": "Send the owner a weekly summary on Fridays.", "purpose": "preference", "request_id": "owner-edit-1",
	})
	if status != http.StatusOK {
		t.Fatalf("retry of an acknowledged save must replay: %d", status)
	}
	status, _ = ownerJSON(t, server, "/visualizer/api/update", map[string]any{
		"id": observation.ID, "expected_revision": observation.Revision, "title": "Weekly summary",
		"content": "Something else entirely.", "purpose": "preference", "request_id": "owner-edit-1",
	})
	if status != http.StatusConflict {
		t.Fatalf("a request ID must not carry a different payload: %d", status)
	}
	if now, _ := store.RecordByID(context.Background(), observation.ID, nil); now.Revision != 2 {
		t.Fatalf("replay or conflict wrote another revision: %+v", now)
	}
}

func observation2project() *string { project := "id:review"; return &project }

func TestOwnerConflictKeepsNewerAgentCorrection(t *testing.T) {
	server, store := testServer(t, true)
	original := agentSave(t, store, "conflict-original", "Backup plan", "Backups go to the local folder.", "decision", true)
	title, project := "Backup plan", "id:review"
	agent := gomemory.WriteInput{Scope: gomemory.Scope{Project: &project}, Title: &title, Content: "Backups go to the encrypted off-host archive.", Purpose: "decision", Confirmed: true,
		Provenance: gomemory.Provenance{Harness: "cursor", Device: "mac-mini", Source: "agent correction"}, RequestID: "conflict-agent", ID: &original.ID, ExpectedRevision: &original.Revision}
	if _, err := store.Write(context.Background(), agent, nil, ""); err != nil {
		t.Fatal(err)
	}
	status, body := ownerJSON(t, server, "/visualizer/api/update", map[string]any{
		"id": original.ID, "expected_revision": original.Revision, "title": "Backup plan",
		"content": "Backups go to a USB drive.", "purpose": "decision", "request_id": "conflict-owner",
	})
	if status != http.StatusConflict {
		t.Fatalf("stale owner edit status=%d body=%s", status, body)
	}
	var current gomemory.Record
	if err := json.Unmarshal(body["current"], &current); err != nil || current.Revision != 2 || current.Content != agent.Content {
		t.Fatalf("conflict must return the newer text: %+v err=%v", current, err)
	}
	if now, _ := store.RecordByID(context.Background(), original.ID, nil); now.Content != agent.Content || now.Revision != 2 {
		t.Fatalf("stale edit overwrote the newer correction: %+v", now)
	}
}

func TestOwnerArchivesAndRestoresAMemory(t *testing.T) {
	server, store := testServer(t, true)
	receipt := agentSave(t, store, "archive-original", "Retire me", "A decision that no longer applies.", "decision", true)
	titles, _ := listed(t, server, `{"all_projects":true}`)
	if !titles["Retire me"] {
		t.Fatalf("active list missing the memory: %v", titles)
	}
	status, body := ownerJSON(t, server, "/visualizer/api/archive", map[string]any{"id": receipt.ID, "expected_revision": receipt.Revision, "archived": true, "request_id": "owner-archive"})
	if status != http.StatusOK {
		t.Fatalf("archive status=%d body=%s", status, body)
	}
	titles, counts := listed(t, server, `{"all_projects":true}`)
	if titles["Retire me"] || counts["archived"] != 1 {
		t.Fatalf("archived memory still active: %v %v", titles, counts)
	}
	if archived, _ := listed(t, server, `{"all_projects":true,"view":"archived"}`); !archived["Retire me"] {
		t.Fatalf("archived list missing the memory: %v", archived)
	}
	agent, err := store.Context(context.Background(), gomemory.Scope{Project: observation2project()}, 16384)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range agent.Records {
		if record.ID == receipt.ID {
			t.Fatal("archived memory still loads for agents")
		}
	}
	status, _ = ownerJSON(t, server, "/visualizer/api/update", map[string]any{"id": receipt.ID, "expected_revision": 2, "title": "Retire me", "content": "Edited while archived.", "purpose": "decision", "request_id": "owner-edit-archived"})
	if status != http.StatusConflict {
		t.Fatalf("an archived memory must be restored before it is edited: %d", status)
	}
	status, _ = ownerJSON(t, server, "/visualizer/api/archive", map[string]any{"id": receipt.ID, "expected_revision": 1, "archived": false, "request_id": "owner-stale-restore"})
	if status != http.StatusConflict {
		t.Fatalf("stale restore status=%d", status)
	}
	status, _ = ownerJSON(t, server, "/visualizer/api/archive", map[string]any{"id": receipt.ID, "expected_revision": 2, "archived": false, "request_id": "owner-restore"})
	if status != http.StatusOK {
		t.Fatalf("restore status=%d", status)
	}
	restored, _ := store.RecordByID(context.Background(), receipt.ID, nil)
	if restored.Archived || restored.Revision != 3 || restored.Content != "A decision that no longer applies." || !restored.Confirmed {
		t.Fatalf("restore changed more than visibility: %+v", restored)
	}
	one := int64(1)
	if first, _ := store.RecordByID(context.Background(), receipt.ID, &one); first == nil || first.Archived {
		t.Fatalf("history lost: %+v", first)
	}
}

func TestOwnerWritesNeedAnOwnerSessionAndExactOrigin(t *testing.T) {
	server, store := testServer(t, true)
	receipt := agentSave(t, store, "auth-original", "Guarded", "Only the owner may change this.", "decision", true)
	paired := "synthetic-paired-device-token-0123456789"
	if _, err := store.AddClientToken(context.Background(), "paired", sha256.Sum256([]byte(paired))); err != nil {
		t.Fatal(err)
	}
	key := sha256.Sum256([]byte(testToken))
	expired, _ := newVisualizerSession(key[:], time.Now().Add(-time.Second))
	update, _ := json.Marshal(map[string]any{"id": receipt.ID, "expected_revision": 1, "title": "Guarded", "content": "Changed.", "purpose": "decision", "request_id": "auth-attempt"})
	archive, _ := json.Marshal(map[string]any{"id": receipt.ID, "expected_revision": 1, "archived": true, "request_id": "auth-archive"})
	for _, route := range []struct{ path, body string }{{"/visualizer/api/update", string(update)}, {"/visualizer/api/archive", string(archive)}} {
		for _, tc := range []struct {
			name, token, origin, cookie string
			status                      int
		}{
			{"anonymous", "", server.URL, "", 401},
			{"paired device", paired, server.URL, "", 401},
			{"foreign origin", testToken, "https://other.example", "", 403},
			{"missing origin", testToken, "", "", 403},
			{"expired session", "", server.URL, expired, 401},
		} {
			req := httptest.NewRequest(http.MethodPost, server.URL+route.path, strings.NewReader(route.body))
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: visualizerCookieName, Value: tc.cookie})
			}
			response := httptest.NewRecorder()
			server.Config.Handler.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("%s %s: status=%d want %d", route.path, tc.name, response.Code, tc.status)
			}
		}
	}
	unchanged, _ := store.RecordByID(context.Background(), receipt.ID, nil)
	if unchanged.Revision != 1 || unchanged.Archived || unchanged.Content != "Only the owner may change this." {
		t.Fatalf("a rejected request changed the memory: %+v", unchanged)
	}
	for _, body := range []string{
		`{"id":1,"expected_revision":1,"title":"","content":"x","purpose":"decision","request_id":"r","scope":{"project":"id:other"}}`,
		`{"id":0,"expected_revision":1,"title":"","content":"x","purpose":"decision","request_id":"r"}`,
		`{"id":1,"expected_revision":0,"title":"","content":"x","purpose":"decision","request_id":"r"}`,
	} {
		if status, _ := ownerRead(t, server, "/visualizer/api/update", body); status != http.StatusBadRequest {
			t.Fatalf("invalid update accepted: %s -> %d", body, status)
		}
	}
	if status, _ := ownerRead(t, server, "/visualizer/api/update", `{"id":`+jsonNumber(receipt.ID)+`,"expected_revision":1,"title":"","content":"   ","purpose":"decision","request_id":"blank"}`); status != http.StatusBadRequest {
		t.Fatalf("blank content accepted: %d", status)
	}
	if status, _ := ownerRead(t, server, "/visualizer/api/update", `{"id":9999,"expected_revision":1,"title":"","content":"x","purpose":"decision","request_id":"missing"}`); status != http.StatusNotFound {
		t.Fatalf("missing memory status=%d", status)
	}
}

func TestOwnerEditIsEmbeddedForSearchOrRefusedWholeWhenTheModelFails(t *testing.T) {
	_, store := testServer(t)
	project := "id:review"
	title := "Embedded"
	receipt, err := store.Write(context.Background(), gomemory.WriteInput{Scope: gomemory.Scope{Project: &project}, Title: &title, Content: "Original wording.", Purpose: "decision", Confirmed: true,
		Provenance: gomemory.Provenance{Harness: "test", Device: "test", Source: "embedding regression"}, RequestID: "embed-original"}, []float32{1, 0}, "owner-test")
	if err != nil {
		t.Fatal(err)
	}
	edit := func(embedder Embedder, request, content string, revision int64) int {
		handler, err := NewHandler(Backend{Store: store, Visualizer: true, Embedder: embedder, Model: "owner-test"}, testToken)
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(handler)
		defer server.Close()
		status, _ := ownerJSON(t, server, "/visualizer/api/update", map[string]any{"id": receipt.ID, "expected_revision": revision, "title": "Embedded", "content": content, "purpose": "decision", "request_id": request})
		return status
	}
	if status := edit(failedEmbedder{}, "embed-fail", "Unsaved wording.", 1); status != http.StatusServiceUnavailable {
		t.Fatalf("a failed model must refuse the save: %d", status)
	}
	if now, _ := store.RecordByID(context.Background(), receipt.ID, nil); now.Revision != 1 || now.Content != "Original wording." {
		t.Fatalf("a refused save changed the memory: %+v", now)
	}
	if status := edit(constantEmbedder{}, "embed-ok", "Rewritten wording.", 1); status != http.StatusOK {
		t.Fatalf("save status=%d", status)
	}
	page, err := store.Search(context.Background(), gomemory.Scope{Project: &project}, "unrelated words", []float32{0, 1}, "owner-test", 5, 16384)
	if err != nil || len(page.Records) != 1 || page.Records[0].Content != "Rewritten wording." {
		t.Fatalf("the corrected memory is not semantically searchable at once: %+v err=%v", page, err)
	}
}
