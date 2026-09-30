package gomemory

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestStartupPreviewAccountsForEveryActiveScopedMemory(t *testing.T) {
	// Given eligible decisions, unconfirmed observations and multiple handoffs,
	// every active in-scope memory is loaded or explained exactly once. Once the
	// detail cap is reached, the exact total still accounts for all active rows.
	for _, unconfirmedCount := range []int{3, 130} {
		t.Run(fmt.Sprintf("unconfirmed-%d", unconfirmedCount), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "startup.db")
			if err := CreateEmpty(path); err != nil {
				t.Fatal(err)
			}
			w, err := OpenWritableExisting(path, "synthetic", 2)
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			ctx := context.Background()
			scope := Scope{Project: ptr("id:startup-snapshot")}
			active := map[int64]bool{}
			save := func(label, purpose string, confirmed bool, stored Scope, counts bool) Receipt {
				receipt, err := w.Write(ctx, WriteInput{Scope: stored, Content: label, Purpose: purpose, Confirmed: confirmed, Provenance: Provenance{"test", "synthetic", "startup snapshot accounting"}, RequestID: label}, nil, "")
				if err != nil {
					t.Fatal(err)
				}
				if counts {
					active[receipt.ID] = true
				}
				return receipt
			}
			save("confirmed decision", "decision", true, scope, true)
			save("confirmed preference", "preference", true, Scope{}, true)
			save("global handoff", "handoff", false, Scope{}, true)
			save("old project handoff", "handoff", false, scope, true)
			projectHandoff := save("new project handoff", "handoff", false, scope, true)
			for i := 0; i < unconfirmedCount; i++ {
				save(fmt.Sprintf("observation-%d", i), "observation", false, scope, true)
			}
			save("other project's decision", "decision", true, Scope{Project: ptr("id:other-startup")}, false)
			archived := save("archived scoped observation", "observation", false, scope, false)
			if _, err := w.Archive(ctx, ArchiveInput{Scope: scope, ID: archived.ID, ExpectedRevision: 1, Archived: true, RequestID: "archive-startup-observation", Provenance: Provenance{"test", "synthetic", "startup boundary"}}); err != nil {
				t.Fatal(err)
			}
			preview, err := w.StartupPreview(ctx, scope, 12000)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Records)+preview.NotLoadedTotal != len(active) {
				t.Fatalf("accounting loaded %d exclusions %d active %d", len(preview.Records), preview.NotLoadedTotal, len(active))
			}
			seen := map[int64]bool{}
			for _, record := range preview.Records {
				if seen[record.ID] || !active[record.ID] {
					t.Fatalf("loaded duplicate/out-of-scope %d", record.ID)
				}
				seen[record.ID] = true
			}
			if !seen[projectHandoff.ID] {
				t.Fatal("latest project handoff not loaded")
			}
			for _, record := range preview.NotLoaded {
				if seen[record.ID] || !active[record.ID] {
					t.Fatalf("excluded duplicate/out-of-scope %d", record.ID)
				}
				seen[record.ID] = true
			}
			if unconfirmedCount < 100 {
				if len(seen) != len(active) {
					t.Fatalf("not all records represented: got %d want %d", len(seen), len(active))
				}
			} else if len(preview.NotLoaded) != 100 {
				t.Fatalf("detail cap changed: %d", len(preview.NotLoaded))
			}
			agent, err := w.Context(ctx, scope, 12000)
			if err != nil {
				t.Fatal(err)
			}
			if len(agent.Records) != len(preview.Records) {
				t.Fatal("preview selected count differs from agent context")
			}
			for i, record := range agent.Records {
				if preview.Records[i].ID != record.ID {
					t.Fatal("preview changed priority/order")
				}
			}
		})
	}
}
