package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func TestPrepareQuickstartBackupPreservesSource(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Grasshopper")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "memory.db")
	if err := gomemory.CreateEmpty(source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "access-token"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := prepareQuickstartBackup(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareQuickstartBackup(dir)
	if err != nil || first == second {
		t.Fatalf("backup was replaced: %s %s %v", first, second, err)
	}
	for _, path := range []string{source, first, second} {
		reader, err := gomemory.OpenReadOnly(path)
		if err != nil {
			t.Fatalf("snapshot %s unavailable: %v", path, err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("snapshot not private: %s: %v", path, err)
			}
		}
	}
}

func TestPrepareQuickstartBackupRefusesPartialState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "access-token"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareQuickstartBackup(dir); err == nil {
		t.Fatal("partial state accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "backups")); !os.IsNotExist(err) {
		t.Fatal("partial state created backup directory")
	}
}
