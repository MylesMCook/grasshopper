package main

import (
	"github.com/MylesMCook/grasshopper/internal/memory"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVerifyBackupReportsIntegrityWithoutChangingSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.db")
	if err := memory.CreateEmpty(path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	result, err := verifyBackup(path)
	if err != nil || result.Records != 0 || result.Revisions != 0 || result.QuickCheck != "ok" || len(result.SHA256) != 64 || result.Size == 0 {
		t.Fatalf("invalid report: %+v %v", result, err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("verification changed snapshot")
	}
	for _, data := range [][]byte{nil, []byte("corrupt")} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := verifyBackup(path); err == nil {
			t.Fatal("invalid database verified")
		}
	}
	if runtime.GOOS != "windows" {
		if err := memory.CreateEmpty(filepath.Join(t.TempDir(), "unused")); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := verifyBackup(path); err == nil {
			t.Fatal("shared snapshot accepted")
		}
	}
}

func TestVerifyBackupRejectsPendingSidecars(t *testing.T) {
	for _, suffix := range []string{"-wal", "-journal"} {
		t.Run(suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot.db")
			if err := memory.CreateEmpty(path); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+suffix, []byte("pending transaction"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := verifyBackup(path); err == nil {
				t.Fatal("sidecar-bearing file verified as a standalone snapshot")
			}
		})
	}
}
