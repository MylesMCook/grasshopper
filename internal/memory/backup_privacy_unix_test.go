//go:build !windows

package memory

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestBackupStaysPrivateDuringCopy(t *testing.T) {
	// Run without t.Parallel: umask is process-wide.
	previous := syscall.Umask(0022)
	defer syscall.Umask(previous)
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "source.db"), filepath.Join(dir, "backup.db")
	if err := CreateEmpty(source); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWritableExisting(source, "synthetic", 1)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the copy in progress long enough to observe the populated snapshot.
	_, fillErr := w.db.Exec("CREATE TABLE padding(data BLOB); INSERT INTO padding VALUES(zeroblob(33554432))")
	closeErr := w.Close()
	if fillErr != nil || closeErr != nil {
		t.Fatalf("prepare synthetic database: %v, %v", fillErr, closeErr)
	}
	done := make(chan error, 1)
	go func() {
		copy, err := OpenWritableCopy(context.Background(), source, destination)
		if err == nil {
			err = copy.Close()
		}
		done <- err
	}()
	observed := false
	var exposed os.FileMode
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			if !observed {
				t.Fatal("copy completed without observing the snapshot; privacy was not verified")
			}
			if exposed != 0 {
				t.Fatalf("snapshot readable during copy: mode %04o", exposed)
			}
			return
		default:
			paths, err := filepath.Glob(filepath.Join(dir, ".backup.db.tmp-*"))
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range paths {
				info, err := os.Stat(path)
				if err == nil && info.Size() > 0 {
					observed = true
					if info.Mode().Perm()&0077 != 0 {
						exposed = info.Mode().Perm()
					}
				}
			}
			time.Sleep(100 * time.Microsecond)
		}
	}
}
