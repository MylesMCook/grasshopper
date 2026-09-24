package gomemory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateEmptyMemoryDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	if err := CreateEmpty(path); err != nil {
		t.Fatal(err)
	}
	if err := CreateEmpty(path); !os.IsExist(err) {
		t.Fatalf("existing database was not protected: %v", err)
	}
	w, err := OpenWritableExisting(path, "test-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	key := "answer-style"
	input := WriteInput{Scope: Scope{}, Content: "Use a short answer with a source.", Purpose: "preference", Confirmed: true,
		Provenance: Provenance{"codex", "test-mac", "synthetic first-run test"}, RequestID: "fresh-write", Key: &key}
	receipt, err := w.Write(context.Background(), input, []float32{1, 0}, "test-model")
	if err != nil || receipt.ID != 1 || receipt.Revision != 1 {
		t.Fatalf("fresh write: %+v %v", receipt, err)
	}
	page, err := w.Search(context.Background(), Scope{}, "short answer", []float32{1, 0}, "test-model", 10, 4000)
	if err != nil || len(page.Records) != 1 || page.Records[0].ID != receipt.ID {
		t.Fatalf("fresh recall: %+v %v", page, err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWritableExisting(path, "test-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	record, err := reopened.Get(context.Background(), Scope{}, receipt.ID, nil)
	if err != nil || record == nil || record.Content != input.Content {
		t.Fatalf("reopened record: %+v %v", record, err)
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	copy, err := OpenWritableCopy(context.Background(), path, backup)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	fromBackup, err := copy.Get(context.Background(), Scope{}, receipt.ID, nil)
	if err != nil || fromBackup == nil || fromBackup.Revision != 1 {
		t.Fatalf("backup record: %+v %v", fromBackup, err)
	}
}
