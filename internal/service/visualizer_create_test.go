package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/memory"
)

// The owner adds a kept memory for every project or one durable project; a
// retry replays, and empty text or a folder-name project saves nothing.
func TestOwnerCreatesAMemory(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	create := map[string]any{"project": nil, "title": "Short answers", "content": "Prefer short answers.", "request_id": "create-1"}
	status, body := ownerJSON(t, server, "/visualizer/api/create", create)
	if status != http.StatusOK {
		t.Fatalf("create status=%d body=%v", status, body)
	}
	var id int64
	_ = json.Unmarshal(body["id"], &id)
	saved, err := store.RecordByID(ctx, id, nil)
	if err != nil || saved == nil || saved.Scope.Project != nil || !saved.Confirmed || saved.Purpose != "preference" || saved.Title != "Short answers" || saved.Provenance.Harness != "memory-view" {
		t.Fatalf("created memory is wrong: %+v %v", saved, err)
	}
	if status, retry := ownerJSON(t, server, "/visualizer/api/create", create); status != http.StatusOK || string(retry["id"]) != string(body["id"]) {
		t.Fatalf("retry status=%d body=%v", status, retry)
	}
	if status, _ := ownerJSON(t, server, "/visualizer/api/create", map[string]any{"project": nil, "title": "Short answers", "content": "Changed.", "request_id": "create-1"}); status != http.StatusConflict {
		t.Fatalf("a reused request ID with other text: status=%d", status)
	}
	status, body = ownerJSON(t, server, "/visualizer/api/create", map[string]any{"project": "id:review", "title": "", "content": "Run go vet before a PR.", "request_id": "create-2"})
	_ = json.Unmarshal(body["id"], &id)
	if saved, _ = store.RecordByID(ctx, id, nil); status != http.StatusOK || saved == nil || saved.Scope.Project == nil || *saved.Scope.Project != "id:review" {
		t.Fatalf("project create status=%d record=%+v", status, saved)
	}
	// Text an agent already saved under a key here is a duplicate, not a server failure.
	key := "keyed-rule"
	if _, err := store.Write(ctx, memory.WriteInput{Scope: memory.Scope{}, Title: ptr("Keyed"), Content: "Keyed rule.", Purpose: "preference", Confirmed: true, Key: &key,
		Provenance: memory.Provenance{Harness: "codex-desktop", Device: "mac", Source: "keyed"}, RequestID: "keyed-1"}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if status, body := ownerJSON(t, server, "/visualizer/api/create", map[string]any{"project": nil, "title": "Keyed", "content": "Keyed rule.", "request_id": "create-dup"}); status != http.StatusConflict || string(body["error"]) != `"duplicate_conflict"` {
		t.Fatalf("duplicate create: status=%d body=%v", status, body)
	}
	for name, input := range map[string]map[string]any{
		"empty text":          {"project": nil, "title": "x", "content": "  ", "request_id": "create-empty"},
		"folder-name project": {"project": "plain-folder-name", "title": "x", "content": "x", "request_id": "create-legacy"},
		"empty project":       {"project": "", "title": "x", "content": "x", "request_id": "create-empty-project"},
	} {
		if status, _ := ownerJSON(t, server, "/visualizer/api/create", input); status != http.StatusBadRequest {
			t.Fatalf("%s: status=%d", name, status)
		}
	}
}
