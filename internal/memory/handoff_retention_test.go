package memory

import (
	"context"
	"strings"
	"testing"
)

// A new handoff archives the earlier one in the same exact scope and leaves
// other scopes alone; the replaced handoff stays readable in history.
func TestNewHandoffRetiresTheEarlierOneInItsScope(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	alpha, beta := "id:alpha", "id:beta"
	save := func(project *string, text, request string) Receipt {
		t.Helper()
		receipt, err := w.Write(ctx, WriteInput{
			Scope: Scope{Project: project}, Content: text, Purpose: "handoff",
			Provenance: Provenance{"codex", "synthetic-mac", "end of session"}, RequestID: request,
		}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	first := save(&alpha, "Alpha: shipped the parser. Next: tests.", "handoff-a1")
	other := save(&beta, "Beta: halfway through the migration.", "handoff-b1")
	second := save(&alpha, "Alpha: tests pass. Next: release.", "handoff-a2")

	replaced, err := w.RecordByID(ctx, first.ID, nil)
	if err != nil || replaced == nil || !replaced.Archived || replaced.Revision != 2 || !strings.HasPrefix(replaced.Provenance.Source, "Replaced by handoff") {
		t.Fatalf("earlier handoff was not archived with a replacement note: %+v %v", replaced, err)
	}
	original, err := w.RecordByID(ctx, first.ID, &first.Revision)
	if err != nil || original == nil || original.Content != "Alpha: shipped the parser. Next: tests." {
		t.Fatalf("replaced handoff lost its original text: %+v %v", original, err)
	}
	for _, check := range []struct {
		project *string
		id      int64
	}{{&alpha, second.ID}, {&beta, other.ID}} {
		current, err := w.Get(ctx, Scope{Project: check.project}, check.id, nil)
		if err != nil || current == nil || current.Archived {
			t.Fatalf("current handoff %d is not active: %+v %v", check.id, current, err)
		}
	}

	// A retried request replays its receipt without retiring anything again.
	again := save(&alpha, "Alpha: tests pass. Next: release.", "handoff-a2")
	if again != second {
		t.Fatalf("retry changed the receipt: %+v vs %+v", again, second)
	}
}

// Restoring an older handoff makes it current and archives the newer one, so
// a scope never holds two active handoffs.
func TestRestoringAHandoffKeepsOnePerScope(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project := "id:restore"
	scope := Scope{Project: &project}
	save := func(text, request string) Receipt {
		t.Helper()
		receipt, err := w.Write(ctx, WriteInput{Scope: scope, Content: text, Purpose: "handoff", Provenance: Provenance{"codex", "synthetic-mac", "end of session"}, RequestID: request}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	first := save("First session note.", "restore-1")
	second := save("Second session note.", "restore-2")
	older, _ := w.RecordByID(ctx, first.ID, nil)
	if _, err := w.Archive(ctx, ArchiveInput{Scope: scope, ID: first.ID, ExpectedRevision: older.Revision, Archived: false, RequestID: "restore-older", Provenance: Provenance{"memory-view", "owner browser", "restore"}}); err != nil {
		t.Fatal(err)
	}
	restored, _ := w.RecordByID(ctx, first.ID, nil)
	newer, _ := w.RecordByID(ctx, second.ID, nil)
	if restored.Archived || !newer.Archived {
		t.Fatalf("restore left two or zero active handoffs: restored archived=%v newer archived=%v", restored.Archived, newer.Archived)
	}
}

// A project handoff applies to the whole project: a device or platform the
// agent sends is dropped, so another machine on the same project sees it, and
// it replaces handoffs saved per device before this rule existed.
func TestProjectHandoffIgnoresDeviceAndReplacesDeviceHandoffs(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project, mac, windows, macos := "id:shared", "synthetic-mac", "synthetic-pc", "macos"
	save := func(device *string, text, request string) Receipt {
		t.Helper()
		receipt, err := w.Write(ctx, WriteInput{
			Scope: Scope{Project: &project, Device: device, Platform: &macos}, Content: text, Purpose: "handoff",
			Provenance: Provenance{"codex", "synthetic", "end of session"}, RequestID: request,
		}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}

	// Simulate a handoff stored per device by an earlier release.
	old := save(&mac, "Mac: left the parser half done.", "device-1")
	oldScope, _ := Scope{Project: &project, Device: &mac, Platform: &macos}.Key()
	if _, err := w.db.ExecContext(ctx, "UPDATE chunks SET memory_scope=? WHERE id=?", oldScope, old.ID); err != nil {
		t.Fatal(err)
	}

	current := save(&windows, "PC: parser done. Next: tests.", "device-2")
	record, err := w.Get(ctx, Scope{Project: &project}, current.ID, nil)
	if err != nil || record == nil || record.Scope.Device != nil || record.Scope.Platform != nil {
		t.Fatalf("handoff kept a device or platform scope: %+v %v", record, err)
	}
	stale, err := w.RecordByID(ctx, old.ID, nil)
	if err != nil || stale == nil || !stale.Archived {
		t.Fatalf("device handoff for the same project stayed active: %+v %v", stale, err)
	}
}

// Handoffs stay short: over 600 characters is refused with guidance, counted
// in characters rather than bytes.
func TestHandoffLongerThan600CharactersIsRefused(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project := "id:short"
	save := func(text, request string) error {
		_, err := w.Write(ctx, WriteInput{
			Scope: Scope{Project: &project}, Content: text, Purpose: "handoff",
			Provenance: Provenance{"codex", "synthetic-mac", "end of session"}, RequestID: request,
		}, nil, "")
		return err
	}
	if err := save(strings.Repeat("é", 600), "limit-600"); err != nil {
		t.Fatalf("600 characters should fit: %v", err)
	}
	err := save(strings.Repeat("a", 601), "limit-601")
	if err == nil || !strings.Contains(err.Error(), "600") {
		t.Fatalf("601 characters should be refused with the limit named: %v", err)
	}
	// The limit is for handoffs only.
	if _, err := w.Write(ctx, WriteInput{
		Scope: Scope{Project: &project}, Content: strings.Repeat("a", 601), Purpose: "decision",
		Provenance: Provenance{"codex", "synthetic-mac", "design review"}, RequestID: "long-decision",
	}, nil, ""); err != nil {
		t.Fatalf("a long decision should still save: %v", err)
	}
}

// Restoring a handoff stored per device by an earlier release must not hide
// the project-wide handoff from every other device.
func TestRestoringADeviceHandoffKeepsTheProjectHandoff(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project, mac := "id:restore-device", "synthetic-mac"
	deviceScope := Scope{Project: &project, Device: &mac}
	save := func(text, request string) Receipt {
		t.Helper()
		receipt, err := w.Write(ctx, WriteInput{Scope: Scope{Project: &project}, Content: text, Purpose: "handoff", Provenance: Provenance{"codex", "synthetic", "end of session"}, RequestID: request}, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	old := save("Mac: left the parser half done.", "legacy-device")
	key, _ := deviceScope.Key()
	if _, err := w.db.ExecContext(ctx, "UPDATE chunks SET memory_scope=? WHERE id=?", key, old.ID); err != nil {
		t.Fatal(err)
	}
	current := save("PC: parser done. Next: tests.", "project-wide")
	retired, _ := w.RecordByID(ctx, old.ID, nil)
	if !retired.Archived {
		t.Fatal("setup: the project handoff should retire the device handoff")
	}

	if _, err := w.Archive(ctx, ArchiveInput{Scope: deviceScope, ID: old.ID, ExpectedRevision: retired.Revision, Archived: false, RequestID: "restore-device", Provenance: Provenance{"memory-view", "owner browser", "restore"}}); err != nil {
		t.Fatal(err)
	}
	projectWide, _ := w.RecordByID(ctx, current.ID, nil)
	if projectWide.Archived {
		t.Fatal("restoring a device handoff archived the project-wide handoff other devices load")
	}
}
