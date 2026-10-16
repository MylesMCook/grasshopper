package memory

import (
	"context"
	"strings"
	"testing"
)

var testProvenance = Provenance{"fixture", "synthetic-mac", "synthetic correction test"}

func TestCorrectionKeepsMetadataItDoesNotResend(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	title, tags, kind := "Release steps", "ci,release", "procedure"
	saved, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "Tag the release, then push.", Title: &title, Tags: &tags, MemoryType: &kind, Purpose: "decision", Confirmed: true, Provenance: testProvenance, RequestID: "meta-1"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(ctx, WriteInput{Scope: Scope{}, ID: &saved.ID, ExpectedRevision: &saved.Revision, Content: "Tag, then push the tag.", Purpose: "decision", Confirmed: true, Provenance: testProvenance, RequestID: "meta-2"}, nil, ""); err != nil {
		t.Fatal(err)
	}
	got, err := w.Get(ctx, Scope{}, saved.ID, nil)
	if err != nil || got == nil {
		t.Fatalf("corrected record: %+v %v", got, err)
	}
	if got.Title != title || got.Tags != tags || got.MemoryType != kind || got.Content != "Tag, then push the tag." {
		t.Fatalf("correction lost metadata: title=%q tags=%q type=%q content=%q", got.Title, got.Tags, got.MemoryType, got.Content)
	}

	// A title derived from the text was never chosen, so it follows new text.
	derived, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "Prefer uv for Python.", Purpose: "decision", Confirmed: true, Provenance: testProvenance, RequestID: "meta-3"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(ctx, WriteInput{Scope: Scope{}, ID: &derived.ID, ExpectedRevision: &derived.Revision, Content: "Prefer uv and uvx for Python.", Purpose: "decision", Confirmed: true, Provenance: testProvenance, RequestID: "meta-4"}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.Get(ctx, Scope{}, derived.ID, nil); got == nil || got.Title != "Prefer uv and uvx for Python." {
		t.Fatalf("derived title did not follow the text: %+v", got)
	}
}

func TestMoveBackRevivesTheOriginalKeyedMemory(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project := "git:github.com/example/moves"
	key := "review-style"
	original, err := w.Write(ctx, WriteInput{Scope: Scope{Project: &project}, Content: "Review with one reviewer.", Purpose: "preference", Confirmed: true, Provenance: testProvenance, RequestID: "move-1", Key: &key}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	moved, err := w.Move(ctx, MoveInput{ID: original.ID, ExpectedRevision: original.Revision, RequestID: "move-2", Provenance: testProvenance}, nil, "")
	if err != nil {
		t.Fatalf("move to every project: %v", err)
	}
	back, err := w.Move(ctx, MoveInput{ID: moved.ID, ExpectedRevision: moved.Revision, Project: &project, RequestID: "move-3", Provenance: testProvenance}, nil, "")
	if err != nil {
		t.Fatalf("move back: %v", err)
	}
	if back.ID != original.ID {
		t.Fatalf("move back made a new record %d instead of reviving %d", back.ID, original.ID)
	}
	got, err := w.Get(ctx, Scope{Project: &project}, original.ID, nil)
	if err != nil || got == nil || got.Archived || got.Content != "Review with one reviewer." {
		t.Fatalf("revived record: %+v %v", got, err)
	}

	// An active memory with the same key in the target stops the move.
	other := "merge-style"
	local, err := w.Write(ctx, WriteInput{Scope: Scope{Project: &project}, Content: "Squash merge here.", Purpose: "preference", Confirmed: true, Provenance: testProvenance, RequestID: "move-4", Key: &other}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "Rebase merge everywhere.", Purpose: "preference", Confirmed: true, Provenance: testProvenance, RequestID: "move-5", Key: &other}, nil, ""); err != nil {
		t.Fatal(err)
	}
	_, err = w.Move(ctx, MoveInput{ID: local.ID, ExpectedRevision: local.Revision, RequestID: "move-6", Provenance: testProvenance}, nil, "")
	if err == nil || !strings.HasPrefix(err.Error(), "move_target_exists") {
		t.Fatalf("move onto an active key: %v", err)
	}
}

func TestDuplicateTextIsATypedConflict(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	write := func(content, request string) Receipt {
		t.Helper()
		receipt, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: content, Purpose: "decision", Confirmed: true, Provenance: testProvenance, RequestID: request}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	first := write("Squash merge only.", "dup-1")
	archived, err := w.Archive(ctx, ArchiveInput{Scope: Scope{}, ID: first.ID, ExpectedRevision: first.Revision, Archived: true, RequestID: "dup-2", Provenance: testProvenance})
	if err != nil {
		t.Fatal(err)
	}
	write("Squash merge only.", "dup-3")
	_, err = w.Archive(ctx, ArchiveInput{Scope: Scope{}, ID: first.ID, ExpectedRevision: archived.Revision, Archived: false, RequestID: "dup-4", Provenance: testProvenance})
	if err == nil || !strings.HasPrefix(err.Error(), "duplicate_conflict") {
		t.Fatalf("restore over an active duplicate: %v", err)
	}

	other := write("Rebase before merging.", "dup-5")
	_, err = w.Write(ctx, WriteInput{Scope: Scope{}, ID: &other.ID, ExpectedRevision: &other.Revision, Content: "Squash merge only.", Purpose: "decision", Confirmed: true, Provenance: testProvenance, RequestID: "dup-6"}, nil, "")
	if err == nil || !strings.HasPrefix(err.Error(), "duplicate_conflict") {
		t.Fatalf("correction onto an active duplicate: %v", err)
	}
}
