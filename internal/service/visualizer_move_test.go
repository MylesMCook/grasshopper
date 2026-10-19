package service

import (
	"context"
	"encoding/json"
	"fmt"
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

// A move only goes to a durable project or every project, and a target that already holds the same memory or a stale revision leaves
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

// A project memory moves straight to another project, keeping its text and
// source, and the original is archived with a note.
func TestOwnerMovesAMemoryBetweenProjects(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	saved := agentSave(t, store, "move-across", "Run vet", "Run go vet before a PR.", "decision", true)
	status, body := ownerJSON(t, server, "/visualizer/api/move", map[string]any{"id": saved.ID, "expected_revision": saved.Revision, "project": "id:elsewhere", "request_id": "move-across-1"})
	if status != http.StatusOK {
		t.Fatalf("move status=%d body=%v", status, body)
	}
	var newID int64
	_ = json.Unmarshal(body["id"], &newID)
	copied, err := store.RecordByID(ctx, newID, nil)
	if err != nil || copied == nil || copied.Scope.Project == nil || *copied.Scope.Project != "id:elsewhere" || copied.Content != "Run go vet before a PR." || copied.Provenance.Harness != "codex-desktop" {
		t.Fatalf("copy in the other project is wrong: %+v %v", copied, err)
	}
	if original, _ := store.RecordByID(ctx, saved.ID, nil); original == nil || !original.Archived {
		t.Fatalf("original was not archived: %+v", original)
	}
}

// The owner changes a device or platform restriction as well as the project.
// A request that leaves device or platform out keeps them as they were.
func TestOwnerChangesWhereADeviceMemoryApplies(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	deviceSave := func(request string) memory.Receipt {
		device, platform := "mac-mini", "macos"
		receipt, err := store.Write(ctx, memory.WriteInput{Scope: memory.Scope{Device: &device, Platform: &platform}, Title: ptr("Worktrees " + request), Content: "Use primary checkouts " + request + ".", Purpose: "preference", Confirmed: true,
			Provenance: memory.Provenance{Harness: "codex-desktop", Device: "mac-mini", Source: "device rule"}, RequestID: request}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	moveTo := func(saved memory.Receipt, body map[string]any) (int, *memory.Record) {
		body["id"], body["expected_revision"], body["request_id"] = saved.ID, saved.Revision, "move-"+strings.ReplaceAll(fmt.Sprint(body), " ", "")
		status, reply := ownerJSON(t, server, "/visualizer/api/move", body)
		if status != http.StatusOK {
			return status, nil
		}
		var id int64
		_ = json.Unmarshal(reply["id"], &id)
		record, _ := store.RecordByID(ctx, id, nil)
		return status, record
	}

	everywhere := deviceSave("a")
	if status, copied := moveTo(everywhere, map[string]any{"project": nil, "device": nil, "platform": nil}); status != http.StatusOK || copied.Scope.Device != nil || copied.Scope.Platform != nil || copied.Provenance.Harness != "codex-desktop" {
		t.Fatalf("lifting the restriction: status=%d %+v", status, copied)
	}
	other := deviceSave("b")
	if status, copied := moveTo(other, map[string]any{"project": nil, "device": "work-laptop", "platform": "windows"}); status != http.StatusOK || *copied.Scope.Device != "work-laptop" || *copied.Scope.Platform != "windows" {
		t.Fatalf("another device: status=%d %+v", status, copied)
	}
	kept := deviceSave("c")
	if status, copied := moveTo(kept, map[string]any{"project": "id:elsewhere"}); status != http.StatusOK || copied.Scope.Device == nil || *copied.Scope.Device != "mac-mini" || *copied.Scope.Project != "id:elsewhere" {
		t.Fatalf("omitted device must stay: status=%d %+v", status, copied)
	}
	same := deviceSave("d")
	if status, _ := moveTo(same, map[string]any{"project": nil, "device": "mac-mini", "platform": "macos"}); status != http.StatusBadRequest {
		t.Fatalf("same place: status=%d", status)
	}
	if status, _ := moveTo(same, map[string]any{"project": nil, "device": nil, "platform": "beos"}); status != http.StatusBadRequest {
		t.Fatalf("invalid platform: status=%d", status)
	}
}

// A moved handoff becomes the current handoff where it lands; the handoff
// that was there is archived as replaced, and both can be restored.
func TestOwnerMovesAHandoff(t *testing.T) {
	server, store := testServer(t, true)
	ctx := context.Background()
	handoff := func(request, device, content string) memory.Receipt {
		receipt, err := store.Write(ctx, memory.WriteInput{Scope: memory.Scope{Device: &device}, Title: ptr(content), Content: content, Purpose: "handoff",
			Provenance: memory.Provenance{Harness: "claude-code-desktop", Device: device, Source: "session"}, RequestID: request}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	laptop := handoff("h-laptop", "work-hp", "Codex on this laptop: Store app only.")
	desk := handoff("h-desk", "mac-mini", "Released 2.10.2.")
	status, body := ownerJSON(t, server, "/visualizer/api/move", map[string]any{"id": laptop.ID, "expected_revision": laptop.Revision, "project": nil, "device": "mac-mini", "platform": nil, "request_id": "move-handoff"})
	if status != http.StatusOK {
		t.Fatalf("move handoff status=%d body=%v", status, body)
	}
	var id int64
	_ = json.Unmarshal(body["id"], &id)
	copied, _ := store.RecordByID(ctx, id, nil)
	if copied == nil || copied.Archived || copied.Purpose != "handoff" || *copied.Scope.Device != "mac-mini" || copied.Provenance.Harness != "claude-code-desktop" {
		t.Fatalf("moved handoff: %+v", copied)
	}
	if replaced, _ := store.RecordByID(ctx, desk.ID, nil); !replaced.Archived || !strings.HasPrefix(replaced.Provenance.Source, "Replaced by handoff") {
		t.Fatalf("the handoff already there should be replaced: %+v", replaced)
	}
	if original, _ := store.RecordByID(ctx, laptop.ID, nil); !original.Archived {
		t.Fatalf("original stays active: %+v", original)
	}
}
