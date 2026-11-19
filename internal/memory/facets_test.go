package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// facetWriter opens an empty database, so only the test's own memories load.
func facetWriter(t *testing.T, areas ...string) *Writer {
	t.Helper()
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := CreateEmpty(path); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWritableExisting(path, "test-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	ctx := context.Background()
	if err := w.EnsureFacetSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, area := range areas {
		if err := w.CreateArea(ctx, area); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func saveFacet(t *testing.T, w *Writer, scope Scope, text, purpose, area string, everywhere bool) Receipt {
	t.Helper()
	input := WriteInput{Scope: scope, Content: text, Purpose: purpose, Confirmed: purpose != "handoff",
		Provenance: Provenance{"test", "synthetic", "facets"}, RequestID: "facet-" + text}
	if area != "" {
		input.Area = &area
	}
	if everywhere {
		input.Everywhere = &everywhere
	}
	receipt, err := w.Write(context.Background(), input, nil, "")
	if err != nil {
		t.Fatalf("save %q: %v", text, err)
	}
	return receipt
}

func loadedText(page Page) string {
	var texts []string
	for _, record := range page.Records {
		texts = append(texts, record.Content)
	}
	return strings.Join(texts, "|")
}

// A global memory with an area loads only where the project draws on that
// area; everywhere and unclassified global memories load in every session.
func TestStartupLoadsGlobalMemoriesByArea(t *testing.T) {
	w := facetWriter(t, "Home", "Work")
	ctx := context.Background()
	house := "id:house"
	saveFacet(t, w, Scope{}, "Home fact.", "decision", "Home", false)
	work := saveFacet(t, w, Scope{}, "Work fact.", "decision", "Work", false)
	saveFacet(t, w, Scope{}, "Unclassified fact.", "decision", "", false)
	saveFacet(t, w, Scope{}, "Everywhere fact.", "decision", "Work", true)
	saveFacet(t, w, Scope{Project: &house}, "House project fact.", "decision", "Work", false)
	if err := w.SetProjectAreas(ctx, house, []string{"Home"}); err != nil {
		t.Fatal(err)
	}

	page, err := w.Context(ctx, Scope{Project: &house}, 16384)
	if err != nil {
		t.Fatal(err)
	}
	loaded := loadedText(page)
	for _, want := range []string{"Home fact.", "Unclassified fact.", "Everywhere fact.", "House project fact."} {
		if !strings.Contains(loaded, want) {
			t.Errorf("project session missing %q: %s", want, loaded)
		}
	}
	if strings.Contains(loaded, "Work fact.") || page.Omitted != 1 || page.OmittedIDs[0] != work.ID {
		t.Errorf("a Work memory leaked into a Home project or was not reported as omitted: %s %+v", loaded, page.OmittedIDs)
	}
	if strings.Join(page.Areas, ",") != "Home,Work" || strings.Join(page.ProjectAreas, ",") != "Home" {
		t.Errorf("startup must list the owner's areas and the project's: %v %v", page.Areas, page.ProjectAreas)
	}

	none, err := w.Context(ctx, Scope{}, 16384)
	if err != nil {
		t.Fatal(err)
	}
	if loaded := loadedText(none); loaded != "Unclassified fact.|Everywhere fact." && loaded != "Everywhere fact.|Unclassified fact." {
		t.Errorf("a session with no project must load only everywhere and unclassified memories: %s", loaded)
	}
}

// Handoffs stop loading after the load window and the owner's preview says why.
func TestStartupSkipsExpiredHandoffs(t *testing.T) {
	w := facetWriter(t)
	ctx := context.Background()
	stale, fresh := "id:stale", "id:fresh"
	old := saveFacet(t, w, Scope{Project: &stale}, "Stale: see tasks.md at abc123.", "handoff", "", false)
	saveFacet(t, w, Scope{Project: &fresh}, "Fresh: see tasks.md at def456.", "handoff", "", false)
	past := time.Now().Add(-handoffLoadWindow - time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := w.db.ExecContext(ctx, "UPDATE chunks SET updated_at=? WHERE id=?", past, old.ID); err != nil {
		t.Fatal(err)
	}
	if page, err := w.Context(ctx, Scope{Project: &stale}, 16384); err != nil || len(page.Records) != 0 || page.Omitted != 1 {
		t.Fatalf("an expired handoff loaded: %+v %v", page, err)
	}
	if page, err := w.Context(ctx, Scope{Project: &fresh}, 16384); err != nil || !strings.Contains(loadedText(page), "Fresh") {
		t.Fatalf("a recent handoff did not load: %+v %v", page, err)
	}
	preview, err := w.StartupPreview(ctx, Scope{Project: &stale}, 16384)
	if err != nil || len(preview.NotLoaded) != 1 || preview.NotLoaded[0].Reason != NotLoadedExpiredHandoff {
		t.Fatalf("preview must name the expired handoff: %+v %v", preview, err)
	}
}

func TestAreaWritesAreValidatedAndKept(t *testing.T) {
	w := facetWriter(t, "Home")
	ctx := context.Background()
	project := "id:garden"
	unknown := "Garden"
	_, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "Typo area.", Purpose: "decision",
		Provenance: Provenance{"test", "synthetic", "facets"}, RequestID: "unknown-area", Area: &unknown}, nil, "")
	if err == nil || !strings.Contains(err.Error(), "unknown_area") || !strings.Contains(err.Error(), "Home") {
		t.Fatalf("an unknown area must be refused with the valid list: %v", err)
	}
	everywhere := true
	_, err = w.Write(ctx, WriteInput{Scope: Scope{Project: &project}, Content: "Project everywhere.", Purpose: "decision",
		Provenance: Provenance{"test", "synthetic", "facets"}, RequestID: "project-everywhere", Area: strPtr("Home"), Everywhere: &everywhere}, nil, "")
	if err == nil || !strings.Contains(err.Error(), "invalid everywhere") {
		t.Fatalf("a project memory cannot load everywhere: %v", err)
	}

	saved := saveFacet(t, w, Scope{}, "Keep my area.", "decision", "Home", false)
	corrected, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "Keep my area, corrected.", Purpose: "decision", Confirmed: true,
		Provenance: Provenance{"test", "synthetic", "facets"}, RequestID: "correct-keeps-area", ID: &saved.ID, ExpectedRevision: &saved.Revision}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	records := []Record{{ID: corrected.ID}}
	if err := w.AttachFacets(ctx, records); err != nil || records[0].Area != "Home" {
		t.Fatalf("a correction without an area must keep the area: %+v %v", records, err)
	}
}

// Duplicate saves leave facets alone, clearing everywhere needs no area, and
// a classified keyed memory can move away and back.
func TestFacetsSurviveDuplicatesAndMoves(t *testing.T) {
	w := facetWriter(t, "Home", "Work")
	ctx := context.Background()
	area := func(id int64) string {
		records := []Record{{ID: id}}
		if err := w.attachFacets(ctx, records); err != nil {
			t.Fatal(err)
		}
		return records[0].Area
	}
	saved := saveFacet(t, w, Scope{}, "Duplicate me.", "decision", "Home", false)
	again, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "Duplicate me.", Purpose: "decision", Confirmed: true,
		Provenance: Provenance{"test", "synthetic", "facets"}, RequestID: "duplicate-new-area", Area: strPtr("Work")}, nil, "")
	if err != nil || !again.Deduplicated || area(saved.ID) != "Home" {
		t.Fatalf("a duplicate save changed the area: %+v %v %q", again, err, area(saved.ID))
	}
	off := false
	if _, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "No area yet.", Purpose: "decision",
		Provenance: Provenance{"test", "synthetic", "facets"}, RequestID: "everywhere-off", Everywhere: &off}, nil, ""); err != nil {
		t.Fatalf("everywhere false without an area must be accepted: %v", err)
	}

	project := "id:garden"
	key := "garden-plan"
	original, err := w.Write(ctx, WriteInput{Scope: Scope{Project: &project}, Content: "Plant in spring.", Purpose: "decision", Confirmed: true,
		Provenance: Provenance{"test", "synthetic", "facets"}, RequestID: "keyed", Key: &key, Area: strPtr("Home")}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	moved, err := w.Move(ctx, MoveInput{ID: original.ID, ExpectedRevision: original.Revision, RequestID: "move-out", Provenance: Provenance{"test", "synthetic", "facets"}}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.db.ExecContext(ctx, "UPDATE memory_areas SET area='Work' WHERE memory_id=?", moved.ID); err != nil {
		t.Fatal(err)
	}
	back, err := w.Move(ctx, MoveInput{ID: moved.ID, ExpectedRevision: moved.Revision, Project: &project, RequestID: "move-back", Provenance: Provenance{"test", "synthetic", "facets"}}, nil, "")
	if err != nil || back.ID != original.ID || area(back.ID) != "Work" {
		t.Fatalf("move back must revive the original with the current area: %+v %v %q", back, err, area(back.ID))
	}
}

func strPtr(value string) *string { return &value }

// Suggestions lists only unkept, non-handoff memories in scope, newest first.
func TestSuggestionsListUnkeptMemoriesNewestFirst(t *testing.T) {
	w := facetWriter(t, "Home")
	ctx := context.Background()
	house, other := "id:house", "id:other"
	saveFacet(t, w, Scope{}, "Kept fact.", "decision", "", false)
	saveFacet(t, w, Scope{Project: &house}, "House handoff: see tasks.md.", "handoff", "", false)
	save := func(scope Scope, text, request string) {
		t.Helper()
		area := "Home"
		if _, err := w.Write(ctx, WriteInput{Scope: scope, Content: text, Purpose: "lesson", Area: &area,
			Provenance: Provenance{"test", "synthetic", "suggestions"}, RequestID: request}, nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	save(Scope{}, "Older suggestion.", "s1")
	save(Scope{Project: &house}, "Newer house suggestion.", "s2")
	save(Scope{Project: &other}, "Other project suggestion.", "s3")
	if _, err := w.db.ExecContext(ctx, "UPDATE chunks SET updated_at='2026-01-01T00:00:00Z' WHERE content='Older suggestion.'"); err != nil {
		t.Fatal(err)
	}
	page, err := w.Suggestions(ctx, Scope{Project: &house}, 10, 16384)
	if err != nil {
		t.Fatal(err)
	}
	if got := loadedText(page); got != "Newer house suggestion.|Older suggestion." {
		t.Fatalf("suggestions must be this scope's unkept memories, newest first: %s", got)
	}
	if page.Records[0].Area != "Home" {
		t.Fatalf("suggestions must carry their proposed area: %+v", page.Records[0])
	}
	if one, _ := w.Suggestions(ctx, Scope{Project: &house}, 1, 16384); len(one.Records) != 1 || one.Omitted != 1 {
		t.Fatalf("the limit must report what it left out: %+v", one)
	}
	if _, err := w.Suggestions(ctx, Scope{}, 0, 16384); err == nil {
		t.Fatal("a zero limit must be refused")
	}
}
