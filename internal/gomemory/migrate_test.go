package gomemory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syntheticEmbedding(content string) ([]float32, error) {
	switch {
	case strings.Contains(content, "bun"):
		return []float32{1, 0}, nil
	case strings.Contains(content, "npm"):
		return []float32{0, 1}, nil
	default:
		return []float32{0.5, 0.5}, nil
	}
}

func TestReembedCopyPreservesRecordsHistoryAndRollbackSource(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "reembedded.db")
	w, err := ReembedCopy(ctx, source, destination, "new-model", syntheticEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	a, b := "id:project-a", "id:project-b"
	for _, scenario := range []struct {
		scope  Scope
		vector []float32
		wantID int64
	}{
		{Scope{Project: &a}, []float32{1, 0}, 2},
		{Scope{Project: &b}, []float32{0, 1}, 3},
	} {
		page, err := w.Search(ctx, scenario.scope, "unmatchedphrase", scenario.vector, "new-model", 10, 16000)
		if err != nil || len(page.Records) == 0 || page.Records[0].ID != scenario.wantID {
			t.Fatalf("migrated scoped search: %+v %v", page, err)
		}
		old, err := w.Search(ctx, scenario.scope, "unmatchedphrase", scenario.vector, "fixture-vector", 10, 16000)
		if err != nil || len(old.Records) != 0 {
			t.Fatalf("old model remained active in migrated copy: %+v %v", old, err)
		}
	}
	current, err := w.Get(ctx, Scope{}, 1, nil)
	if err != nil || current == nil || current.Revision != 2 {
		t.Fatalf("current record changed: %+v %v", current, err)
	}
	previous, err := w.Get(ctx, Scope{}, 1, intPtr(1))
	if err != nil || previous == nil || previous.Revision != 1 {
		t.Fatalf("historical revision lost: %+v %v", previous, err)
	}
	legacy, err := w.Get(ctx, Scope{Legacy: true}, 6, nil)
	if err != nil || legacy == nil || !legacy.Scope.Legacy {
		t.Fatalf("legacy record lost: %+v %v", legacy, err)
	}
	global, err := w.Context(ctx, Scope{}, 16000)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range global.Records {
		if record.ID == 6 {
			t.Fatal("legacy record became global context")
		}
	}
	after, err := os.ReadFile(source)
	if err != nil || string(before) != string(after) {
		t.Fatalf("rollback source changed: %v", err)
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	snapshot, err := OpenWritableCopy(ctx, destination, backup)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	backedUp, err := snapshot.Get(ctx, Scope{}, 1, nil)
	if err != nil || backedUp == nil || backedUp.Revision != 2 {
		t.Fatalf("backup did not restore: %+v %v", backedUp, err)
	}
}

func TestReembedCopyCleansFailedDestination(t *testing.T) {
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	destination := filepath.Join(t.TempDir(), "failed.db")
	count := 0
	_, err := ReembedCopy(context.Background(), source, destination, "new-model", func(content string) ([]float32, error) {
		count++
		if count == 2 {
			return nil, errors.New("synthetic inference failure")
		}
		return syntheticEmbedding(content)
	})
	if err == nil || !strings.Contains(err.Error(), "synthetic inference failure") {
		t.Fatalf("migration failure was hidden: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("failed destination left behind: %v", err)
	}
}

func TestGoServiceReopensOnlyFullyReembeddedDatabase(t *testing.T) {
	ctx := context.Background()
	source := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	oldModelCopy := filepath.Join(t.TempDir(), "old-model.db")
	if err := os.WriteFile(oldModelCopy, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWritableExisting(oldModelCopy, "new-model", 2); err == nil || !strings.Contains(err.Error(), "need shadow re-embedding") {
		t.Fatalf("old-model source was accepted: %v", err)
	}
	destination := filepath.Join(t.TempDir(), "service.db")
	shadow, err := ReembedCopy(ctx, source, destination, "new-model", syntheticEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	if err := shadow.Close(); err != nil {
		t.Fatal(err)
	}
	service, err := OpenWritableExisting(destination, "new-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	decision := WriteInput{Scope: Scope{}, Content: "Synthetic persisted after a Go restart.", Purpose: "observation", Confirmed: false, Provenance: Provenance{Harness: "codex", Device: "synthetic-mac", Source: "restart test"}, RequestID: "persisted-restart"}
	receipt, err := service.Write(ctx, decision, []float32{0.5, 0.5}, "new-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenWritableExisting(destination, "new-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	record, err := restarted.Get(ctx, Scope{}, receipt.ID, nil)
	if err != nil || record == nil || record.Content != decision.Content {
		t.Fatalf("write did not survive restart: %+v %v", record, err)
	}
	missing := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenWritableExisting(missing, "new-model", 2); err == nil {
		t.Fatal("missing database was created")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing database appeared: %v", err)
	}
}

func TestReembedCopyLeavesCodeRowsUntouched(t *testing.T) {
	ctx := context.Background()
	fixture := filepath.Join("..", "..", "tests", "fixtures", "go-compat", "memory.db")
	source := filepath.Join(t.TempDir(), "mixed-source.db")
	w, err := OpenWritableCopy(ctx, fixture, source)
	if err != nil {
		t.Fatal(err)
	}
	blob := vectorBlob([]float32{1, 2})
	inserted, err := w.db.ExecContext(ctx, `INSERT INTO chunks(kind,content,title,created_at,updated_at,embedding,embedding_model)
 VALUES('code','fn example() {}','example','2026-09-23','2026-09-23',?,'old-code-model')`, blob)
	if err != nil {
		t.Fatal(err)
	}
	id, err := inserted.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	shadow, err := ReembedCopy(ctx, source, filepath.Join(t.TempDir(), "mixed-shadow.db"), "new-model", syntheticEmbedding)
	if err != nil {
		t.Fatal(err)
	}
	defer shadow.Close()
	var content, model string
	var kept []byte
	if err := shadow.db.QueryRowContext(ctx, "SELECT content,embedding_model,embedding FROM chunks WHERE id=? AND kind='code'", id).Scan(&content, &model, &kept); err != nil {
		t.Fatal(err)
	}
	if content != "fn example() {}" || model != "old-code-model" || string(kept) != string(blob) {
		t.Fatalf("code row changed: content=%q model=%q embedding=%v", content, model, kept)
	}
}
