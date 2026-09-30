package gomemory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOwnerExportScopeDimensionsAndArchivedReviewUnion(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	one, two, device, otherDevice, mac, windows := "id:export-one", "id:export-two", "export-device", "other-device", "macos", "windows"
	save := func(label string, scope Scope, confirmed bool) Receipt {
		title := label
		receipt, err := w.Write(ctx, WriteInput{Scope: scope, Title: &title, Content: label, Purpose: "observation", Confirmed: confirmed, Provenance: Provenance{"test", "synthetic", "export scope regression"}, RequestID: label}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	global := save("export-global", Scope{}, false)
	selected := save("export-selected", Scope{Project: &one, Device: &device, Platform: &mac}, false)
	otherProject := save("export-other-project", Scope{Project: &two}, false)
	mismatchedDevice := save("export-other-device", Scope{Project: &one, Device: &otherDevice}, false)
	mismatchedPlatform := save("export-other-platform", Scope{Project: &one, Platform: &windows}, false)
	archived := save("export-archived-selected", Scope{Project: &one, Device: &device}, true)
	_, err := w.Archive(ctx, ArchiveInput{Scope: Scope{Project: &one, Device: &device}, ID: archived.ID, ExpectedRevision: 1, Archived: true, RequestID: "export-archived", Provenance: Provenance{"test", "synthetic", "archive fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		scope        BrowseScope
		include      bool
		want, absent []int64
	}{
		{"selected review", BrowseScope{Scope: Scope{Project: &one, Device: &device, Platform: &mac}, View: ViewReview, Purpose: "observation"}, false, []int64{global.ID, selected.ID}, []int64{otherProject.ID, mismatchedDevice.ID, mismatchedPlatform.ID, archived.ID}},
		{"review with archive", BrowseScope{Scope: Scope{Project: &one, Device: &device, Platform: &mac}, View: ViewReview, Purpose: "observation"}, true, []int64{global.ID, selected.ID, archived.ID}, []int64{otherProject.ID, mismatchedDevice.ID, mismatchedPlatform.ID}},
		{"all projects", BrowseScope{AllProjects: true, Purpose: "observation"}, true, []int64{global.ID, selected.ID, otherProject.ID, mismatchedDevice.ID, mismatchedPlatform.ID, archived.ID}, nil},
		{"global", BrowseScope{Purpose: "observation"}, false, []int64{global.ID}, []int64{selected.ID, otherProject.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := map[int64]bool{}
			err := w.WalkOwnerExport(ctx, tc.scope, tc.include, func(record Record) error {
				seen[record.ID] = true
				if record.Scope.Legacy {
					t.Fatal("legacy exposed")
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range tc.want {
				if !seen[id] {
					t.Fatalf("missing%d", id)
				}
			}
			for _, id := range tc.absent {
				if seen[id] {
					t.Fatalf("leaked%d", id)
				}
			}
		})
	}
}

func TestOwnerExportKeepsOneSnapshotWhileNewScopeAndContentCommit(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "export.db")
	w, err := OpenWritableCopy(ctx, filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	title := "before atomic change"
	saved, err := w.Write(ctx, WriteInput{Scope: Scope{}, Content: "before atomic change", Title: &title, Purpose: "observation", Confirmed: true, Provenance: Provenance{"test", "synthetic", "stream snapshot"}, RequestID: "stream-snapshot"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	concurrent, err := openWritableFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer concurrent.Close()
	committed := false
	first := map[string]bool{}
	err = w.WalkOwnerExport(ctx, BrowseScope{AllProjects: true, Purpose: "observation"}, false, func(record Record) error {
		first[record.Title] = true
		if record.ID != saved.ID {
			return nil
		}
		tx, err := concurrent.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.ExecContext(ctx, `UPDATE chunks SET title='after atomic change' WHERE id=?`, saved.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO chunks(kind,content,title,descriptors,memory_type,purpose,confirmed,provenance,content_hash,created_at,updated_at,memory_scope)
 SELECT kind,'new atomic scope','new atomic scope',descriptors,memory_type,purpose,confirmed,provenance,'synthetic-export-new-hash',created_at,updated_at,'{"project":"id:brand-new-export-scope"}' FROM chunks WHERE id=?`, saved.ID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !committed || !first["before atomic change"] || first["after atomic change"] || first["new atomic scope"] {
		t.Fatalf("stream mixed snapshots %v committed%v", first, committed)
	}
	second := map[string]bool{}
	err = w.WalkOwnerExport(ctx, BrowseScope{AllProjects: true, Purpose: "observation"}, false, func(record Record) error { second[record.Title] = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !second["after atomic change"] || !second["new atomic scope"] || second["before atomic change"] {
		t.Fatalf("next export missing atomic change %v", second)
	}
}
