package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/memory"
)

// Moving a memory to every project keeps its text, purpose, confirmation and
// original source, archives the project copy with a note, and a retry with
// the same request ID completes without a second copy.
func TestOwnerMovesAMemoryBetweenAProjectAndEveryProject(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	saved := agentSave(t, store, "move-original", "Short answers", "Prefer short answers.", "preference", true)

	move := map[string]any{"id": saved.ID, "expected_revision": saved.Revision, "project": nil, "request_id": "move-1"}
	status, body := ownerJSON(t, server, "/visualizer/api/move", move)
	if status != http.StatusOK {
		t.Fatalf("move status=%d body=%v", status, body)
	}
	var newID int64
	_ = json.Unmarshal(body["id"], &newID)
	copied, err := store.RecordByID(ctx, newID, nil)
	if err != nil || copied == nil || copied.Scope.Project != nil || copied.Content != "Prefer short answers." || copied.Purpose != "preference" ||
		!copied.Confirmed || copied.Provenance.Harness != "codex-desktop" || copied.Archived {
		t.Fatalf("copy at every project is wrong: %+v %v", copied, err)
	}
	original, err := store.RecordByID(ctx, saved.ID, nil)
	if err != nil || original == nil || !original.Archived || !strings.Contains(original.Provenance.Source, "Moved to memory") {
		t.Fatalf("original was not archived with a move note: %+v %v", original, err)
	}

	status, retry := ownerJSON(t, server, "/visualizer/api/move", move)
	var retryID int64
	_ = json.Unmarshal(retry["id"], &retryID)
	if status != http.StatusOK || retryID != newID {
		t.Fatalf("retry did not replay: status=%d id=%d want %d", status, retryID, newID)
	}

	// A stale revision and a move to the same scope change nothing.
	other := agentSave(t, store, "move-other", "Run vet", "Run go vet before a PR.", "decision", true)
	if status, _ := ownerJSON(t, server, "/visualizer/api/move", map[string]any{"id": other.ID, "expected_revision": other.Revision + 1, "project": nil, "request_id": "move-stale"}); status != http.StatusConflict {
		t.Fatalf("stale move status=%d", status)
	}
	same := "id:review"
	if status, _ := ownerJSON(t, server, "/visualizer/api/move", map[string]any{"id": other.ID, "expected_revision": other.Revision, "project": same, "request_id": "move-same"}); status != http.StatusBadRequest {
		t.Fatalf("same-scope move status=%d", status)
	}
	unchanged, _ := store.RecordByID(ctx, other.ID, nil)
	if unchanged.Archived || unchanged.Revision != other.Revision {
		t.Fatalf("rejected move changed the memory: %+v", unchanged)
	}
}

// A move only goes between a project and every project, to a durable project,
// and a target that already holds the same memory or a stale revision leaves
// everything unchanged, with no stray copy.
func TestRejectedMovesChangeNothing(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	saved := agentSave(t, store, "reject-original", "Short answers", "Prefer short answers.", "preference", true)
	// The same memory already applies to every project.
	if _, err := store.Write(ctx, memory.WriteInput{Scope: memory.Scope{}, Title: ptr("Short answers"), Content: "Prefer short answers.", Purpose: "preference", Confirmed: true,
		Provenance: memory.Provenance{Harness: "claude", Device: "laptop", Source: "earlier"}, RequestID: "reject-global"}, nil, ""); err != nil {
		t.Fatal(err)
	}
	countActive := func() int {
		status, data := ownerRead(t, server, "/visualizer/api/context", `{"all_projects":true}`)
		if status != http.StatusOK {
			t.Fatalf("list status=%d", status)
		}
		var page memory.Page
		_ = json.Unmarshal(data, &page)
		return len(page.Records)
	}
	before := countActive()
	for _, tc := range []struct {
		name    string
		project any
		want    int
	}{
		{"to another project", "id:elsewhere", http.StatusBadRequest},
		{"to a legacy project name", "plain-folder-name", http.StatusBadRequest},
		{"onto an identical global memory", nil, http.StatusConflict},
	} {
		status, _ := ownerJSON(t, server, "/visualizer/api/move", map[string]any{"id": saved.ID, "expected_revision": saved.Revision, "project": tc.project, "request_id": "reject-" + strings.ReplaceAll(tc.name, " ", "-")})
		if status != tc.want {
			t.Fatalf("%s: status=%d want %d", tc.name, status, tc.want)
		}
	}
	original, _ := store.RecordByID(ctx, saved.ID, nil)
	if original.Archived || original.Revision != saved.Revision || countActive() != before {
		t.Fatalf("a rejected move changed state: %+v active %d→%d", original, before, countActive())
	}
}

func ptr(value string) *string { return &value }
