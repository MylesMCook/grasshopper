package gomemory

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func fixtureWriter(t *testing.T) *Writer {
	t.Helper()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	destination := filepath.Join(t.TempDir(), "copy.db")
	w, err := OpenWritableCopy(context.Background(), source, destination)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := OpenWritableCopy(context.Background(), source, destination); err == nil {
		t.Error("writable copy overwrote its destination")
	}
	return w
}

func TestGoWriterReplaysRustAcknowledgement(t *testing.T) {
	w := fixtureWriter(t)
	title, key := "Response style", "answer-style"
	input := WriteInput{
		Scope:   Scope{},
		Content: strings.Repeat("Keep answers concise, but retain a complete source reference. ", 5) + "Only use this preference for the synthetic compatibility test.",
		Title:   &title, Purpose: "preference", Confirmed: true,
		Provenance: Provenance{"fixture", "synthetic-mac", "synthetic Go compatibility fixture"},
		RequestID:  "preference-1", Key: &key,
	}
	receipt, err := w.Write(context.Background(), input, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ID != 1 || receipt.Revision != 1 || receipt.Deduplicated {
		t.Fatalf("Rust replay mismatch: %+v", receipt)
	}
	current, err := w.Get(context.Background(), Scope{}, 1, nil)
	if err != nil || current == nil || current.Revision != 2 {
		t.Fatalf("replay mutated current record: %+v %v", current, err)
	}
}

func TestGoWriterCorrectionAndScopedRecall(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	key := "answer-style"
	input := WriteInput{Scope: Scope{}, Content: "Answer in brief, but keep the complete qualification after character 200. " + strings.Repeat("Source details matter. ", 12), Purpose: "preference", Confirmed: true, Provenance: Provenance{"cursor", "synthetic-work-hp", "approved synthetic correction"}, RequestID: "go-correction-1", Key: &key, ExpectedRevision: intPtr(2)}
	receipt, err := w.Write(ctx, input, []float32{1, 0}, "fixture-vector")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ID != 1 || receipt.Revision != 3 || receipt.Deduplicated {
		t.Fatalf("correction receipt: %+v", receipt)
	}
	current, err := w.Get(ctx, Scope{}, 1, nil)
	if err != nil || current == nil || current.Content != input.Content || current.Revision != 3 {
		t.Fatalf("corrected record: %+v %v", current, err)
	}
	prior, err := w.Get(ctx, Scope{}, 1, intPtr(2))
	if err != nil || prior == nil || prior.Content == current.Content {
		t.Fatalf("prior revision unavailable: %+v %v", prior, err)
	}
	page, err := w.Search(ctx, Scope{}, "qualification", []float32{1, 0}, "fixture-vector", 10, 16000)
	if err != nil || len(page.Records) == 0 || page.Records[0].ID != 1 {
		t.Fatalf("new content not immediately searchable: %+v %v", page, err)
	}
	replayed, err := w.Write(ctx, input, []float32{1, 0}, "fixture-vector")
	if err != nil || replayed != receipt {
		t.Fatalf("replayed write: %+v %v", replayed, err)
	}
	input.RequestID = "go-correction-stale"
	input.Content = "stale content"
	if _, err := w.Write(ctx, input, nil, ""); err == nil || !strings.Contains(err.Error(), "revision_conflict") {
		t.Fatalf("stale correction: %v", err)
	}
	input.RequestID = "go-observation"
	input.ExpectedRevision = intPtr(3)
	input.Confirmed = false
	if _, err := w.Write(ctx, input, nil, ""); err == nil || !strings.Contains(err.Error(), "confirmation_conflict") {
		t.Fatalf("observation replaced confirmation: %v", err)
	}
	still, err := w.Get(ctx, Scope{}, 1, nil)
	if err != nil || still.Revision != 3 {
		t.Fatalf("failed transaction changed record: %+v %v", still, err)
	}
}

func TestGoWriterDoesNotMixProjectDedup(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	projectA, projectB := "id:project-a", "id:project-b"
	base := WriteInput{Scope: Scope{Project: &projectA}, Content: "A scoped reusable lesson.", Purpose: "lesson", Confirmed: true, Provenance: Provenance{"codex", "synthetic-mac", "verified test"}, RequestID: "lesson-a"}
	a, err := w.Write(ctx, base, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	base.RequestID = "lesson-a-second"
	duplicate, err := w.Write(ctx, base, nil, "")
	if err != nil || !duplicate.Deduplicated || duplicate.ID != a.ID {
		t.Fatalf("scoped exact dedup: %+v %v", duplicate, err)
	}
	base.Scope.Project = &projectB
	base.RequestID = "lesson-b"
	b, err := w.Write(ctx, base, nil, "")
	if err != nil || b.ID == a.ID {
		t.Fatalf("cross-project dedup: %+v %v", b, err)
	}
	aPage, err := w.Context(ctx, Scope{Project: &projectA}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range aPage.Records {
		if record.ID == b.ID {
			t.Fatal("project B leaked into project A")
		}
	}
}

func TestWritableDatabaseRejectsBroadUnixPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses file ACLs")
	}
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "memory.db")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := openWritableFile(path); err == nil || !strings.Contains(err.Error(), "other users") {
		t.Fatalf("writable shared database accepted broad permissions: %v", err)
	}
}

func TestProjectWritesNeedDurableIdentityButOlderRecordsRemainReadable(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	local := "folder-on-one-machine"
	input := WriteInput{Scope: Scope{Project: &local}, Content: "Project choice", Purpose: "decision", Confirmed: true, Provenance: Provenance{"codex", "test", "test"}, RequestID: "reject-path"}
	if _, err := w.Write(ctx, input, nil, ""); err == nil || !strings.Contains(err.Error(), "durable") {
		t.Fatalf("accepted a machine-local project identifier: %v", err)
	}
	for _, project := range []string{"id:shared-project", "git:github.com/owner/repo"} {
		input.Scope.Project = &project
		input.RequestID = project
		if _, err := w.Write(ctx, input, nil, ""); err != nil {
			t.Fatalf("rejected durable project %q: %v", project, err)
		}
	}
	// The reader continues to support records made before this validation.
	legacyKey, err := (Scope{Project: &local}).Key()
	if err != nil {
		t.Fatal(err)
	}
	result, err := w.db.ExecContext(ctx, `INSERT INTO chunks(kind,memory_scope,content,title,descriptors,memory_type,purpose,confirmed,provenance,revision,archived,created_at,updated_at)
	 VALUES('memory',?,?,?,?,?,'decision',1,?,1,0,'2026-01-01','2026-01-01')`, legacyKey, "Older choice", "Older", "", "knowledge", `{"harness":"codex","device":"test","source":"older release"}`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	page, err := w.Context(ctx, Scope{Project: &local}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range page.Records {
		found = found || record.ID == id
	}
	if !found {
		t.Fatalf("older project record inaccessible: %+v %v", page, err)
	}
}

func TestViewerBrowsesObservationsAndAllHandoffs(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project := "id:handoff-project"
	scope := Scope{Project: &project}
	base := WriteInput{Scope: scope, Purpose: "handoff", Content: "Project handoff", Confirmed: false, Provenance: Provenance{"codex", "test", "test"}, RequestID: "project-handoff"}
	projectHandoff, err := w.Write(ctx, base, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	base.Scope = Scope{}
	base.Content = "Newer global handoff"
	base.RequestID = "global-handoff"
	globalHandoff, err := w.Write(ctx, base, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	base.Scope = scope
	base.Purpose = "observation"
	base.Content = "Unconfirmed observation"
	base.RequestID = "observation"
	observation, err := w.Write(ctx, base, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	contextPage, err := w.Context(ctx, scope, 16000)
	if err != nil {
		t.Fatal(err)
	}
	seenProject, seenGlobal, seenObservation := false, false, false
	for _, record := range contextPage.Records {
		seenProject = seenProject || record.ID == projectHandoff.ID
		seenGlobal = seenGlobal || record.ID == globalHandoff.ID
		seenObservation = seenObservation || record.ID == observation.ID
	}
	if !seenProject || seenGlobal || seenObservation || contextPage.Omitted == 0 {
		t.Fatalf("agent context should prefer project handoff and disclose omissions: %+v", contextPage)
	}
	browse, _, _, err := w.BrowseContext(ctx, scope, 16000)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{projectHandoff.ID, globalHandoff.ID, observation.ID} {
		found := false
		for _, record := range browse.Records {
			found = found || record.ID == id
		}
		if !found {
			t.Fatalf("active record %d missing from viewer: %+v", id, browse)
		}
	}
}

func TestGoWritableCopyDoesNotChangeSource(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	w := fixtureWriter(t)
	_, err = w.Write(context.Background(), WriteInput{Scope: Scope{}, Content: "Copy only.", Purpose: "observation", Confirmed: false, Provenance: Provenance{"codex", "synthetic-mac", "copy test"}, RequestID: "copy-only"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("source database changed")
	}
}

func TestFailedCopyLeavesNoPartialBackup(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "corrupt.db")
	if err := os.WriteFile(source, []byte("not a SQLite database"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "backup.db")
	if _, err := OpenWritableCopy(context.Background(), source, destination); err == nil {
		t.Fatal("corrupt source copied successfully")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial backup remained: %v", err)
	}
	leftovers, err := filepath.Glob(filepath.Join(directory, ".backup.db.tmp-*"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary backup remained: %v %v", leftovers, err)
	}
}

func TestGoWriterArchiveRestoreAndHistory(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project := "id:project-a"
	scope := Scope{Project: &project}
	provenance := Provenance{"claude", "synthetic-mac", "approved synthetic lifecycle test"}
	archived, err := w.Archive(ctx, ArchiveInput{Scope: scope, ID: 2, ExpectedRevision: 1, Archived: true, RequestID: "archive-a", Provenance: provenance})
	if err != nil || archived.Revision != 2 {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	results, err := w.Search(ctx, scope, "bun", nil, "", 10, 16000)
	if err != nil || len(results.Records) != 0 {
		t.Fatalf("archived record remained active: %+v %v", results, err)
	}
	prior, err := w.Get(ctx, scope, 2, intPtr(1))
	if err != nil || prior == nil || prior.Archived {
		t.Fatalf("prior archive revision: %+v %v", prior, err)
	}
	restored, err := w.Write(ctx, WriteInput{Scope: scope, Content: "", Purpose: "decision", Confirmed: true, Provenance: provenance, RequestID: "restore-a", ID: intPtr(2), ExpectedRevision: intPtr(2), RestoreRevision: intPtr(1)}, nil, "")
	if err != nil || restored.Revision != 3 {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	current, err := w.Get(ctx, scope, 2, nil)
	if err != nil || current == nil || current.Archived || current.Content != "Use bun in project A." {
		t.Fatalf("restored record: %+v %v", current, err)
	}
	results, err = w.Search(ctx, scope, "bun", []float32{1, 0}, "fixture-vector", 10, 16000)
	if err != nil || len(results.Records) == 0 || results.Records[0].ID != 2 {
		t.Fatalf("restored vector not searchable: %+v %v", results, err)
	}
}

func TestRestoreCanReembedAnOldRevisionWithCurrentModel(t *testing.T) {
	w := fixtureWriter(t)
	ctx := context.Background()
	project := "id:project-a"
	scope := Scope{Project: &project}
	provenance := Provenance{"claude", "synthetic-mac", "model migration restore test"}
	archived, err := w.Archive(ctx, ArchiveInput{Scope: scope, ID: 2, ExpectedRevision: 1, Archived: true, RequestID: "archive-before-new-model", Provenance: provenance})
	if err != nil || archived.Revision != 2 {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	restored, err := w.Write(ctx, WriteInput{Scope: scope, Content: "", Purpose: "decision", Confirmed: true, Provenance: provenance, RequestID: "restore-with-new-model", ID: intPtr(2), ExpectedRevision: intPtr(2), RestoreRevision: intPtr(1)}, []float32{0, 1}, "new-model")
	if err != nil || restored.Revision != 3 {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	results, err := w.Search(ctx, scope, "unmatchedphrase", []float32{0, 1}, "new-model", 10, 16000)
	if err != nil || len(results.Records) != 1 || results.Records[0].ID != 2 {
		t.Fatalf("restored record did not use current model: %+v %v", results, err)
	}
}

func TestGoWriterConcurrentCorrectionsConflict(t *testing.T) {
	w1 := fixtureWriter(t)
	var actualPath string
	if err := w1.db.QueryRow("PRAGMA database_list").Scan(new(int), new(string), &actualPath); err != nil {
		t.Fatal(err)
	}
	r2, err := openWritableFile(actualPath)
	if err != nil {
		t.Fatal(err)
	}
	w2 := &Writer{r2}
	defer w2.Close()
	key := "answer-style"
	base := WriteInput{Scope: Scope{}, Purpose: "preference", Confirmed: true, Provenance: Provenance{"codex", "synthetic-mac", "concurrent synthetic test"}, Key: &key, ExpectedRevision: intPtr(2)}
	inputs := []WriteInput{base, base}
	inputs[0].Content = "First concurrent correction"
	inputs[0].RequestID = "concurrent-first"
	inputs[1].Content = "Second concurrent correction"
	inputs[1].RequestID = "concurrent-second"
	inputs[1].Provenance = Provenance{"cursor", "synthetic-windows", "concurrent synthetic test"}
	start := make(chan struct{})
	results := make([]error, 2)
	var wg sync.WaitGroup
	for i, writer := range []*Writer{w1, w2} {
		wg.Add(1)
		go func(i int, writer *Writer) {
			defer wg.Done()
			<-start
			_, results[i] = writer.Write(context.Background(), inputs[i], nil, "")
		}(i, writer)
	}
	close(start)
	wg.Wait()
	success, conflict := 0, 0
	for _, err := range results {
		if err == nil {
			success++
		} else if strings.Contains(err.Error(), "revision_conflict") {
			conflict++
		} else {
			t.Errorf("unexpected concurrent error: %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent writes: success=%d conflict=%d errors=%v", success, conflict, results)
	}
	current, err := w1.Get(context.Background(), Scope{}, 1, nil)
	if err != nil || current == nil || current.Revision != 3 {
		t.Fatalf("concurrent record: %+v %v", current, err)
	}
	if current.Provenance.Harness != "codex" && current.Provenance.Harness != "cursor" {
		t.Fatalf("winning harness provenance missing: %+v", current.Provenance)
	}
}

func TestGoCopyIncludesCommittedWALWrite(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	working := filepath.Join(t.TempDir(), "working.db")
	w, err := OpenWritableCopy(ctx, source, working)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var journalMode string
	if err := w.db.QueryRow("PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil || journalMode != "wal" {
		t.Fatalf("WAL mode: %q %v", journalMode, err)
	}
	input := WriteInput{Scope: Scope{}, Content: "Committed WAL-only synthetic fact", Purpose: "observation", Confirmed: false, Provenance: Provenance{"codex", "synthetic-mac", "WAL backup test"}, RequestID: "wal-write"}
	receipt, err := w.Write(ctx, input, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "snapshot.db")
	copy, err := OpenWritableCopy(ctx, working, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	record, err := copy.Get(ctx, Scope{}, receipt.ID, nil)
	if err != nil || record == nil || record.Content != input.Content {
		t.Fatalf("snapshot omitted committed WAL row: %+v %v", record, err)
	}
}
