package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/MylesMCook/grasshopper/internal/gomemory"
	"io"
	"os"
	"runtime"
	"time"
)

type backupReport struct {
	Records, Revisions, Size int64
	QuickCheck, SHA256       string
}

func verifyBackup(path string) (backupReport, error) {
	var report backupReport
	info, err := os.Lstat(path)
	if err != nil {
		return report, err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return report, errors.New("backup must be a nonempty regular unlinked file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return report, errors.New("backup must be private (mode 0600)")
	}
	// The digest identifies a standalone snapshot, not a live database whose
	// current rows can reside in a WAL or rollback journal.
	for _, suffix := range []string{"-wal", "-journal"} {
		sidecar, err := os.Lstat(path + suffix)
		if err == nil && (!sidecar.Mode().IsRegular() || sidecar.Size() != 0) {
			return report, errors.New("backup has pending WAL/journal state; create a standalone snapshot with --source and --dest, then verify that snapshot")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return report, err
		}
	}
	reader, err := gomemory.OpenReadOnly(path)
	if err != nil {
		return report, err
	}
	defer reader.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report.Records, report.Revisions, report.QuickCheck, err = reader.SnapshotCounts(ctx)
	if err != nil {
		return report, err
	}
	file, err := os.Open(path)
	if err != nil {
		return report, err
	}
	defer file.Close()
	hash := sha256.New()
	report.Size, err = io.Copy(hash, file)
	if err != nil {
		return report, err
	}
	report.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return report, nil
}

func printBackupReport(path string) error {
	report, err := verifyBackup(path)
	if err != nil {
		return fmt.Errorf("verify backup: %w", err)
	}
	fmt.Printf("Verified snapshot: %s\nRecords: %d\nHistorical revisions: %d\nquick_check: %s\nSize: %d bytes\nSHA-256: %s\n", path, report.Records, report.Revisions, report.QuickCheck, report.Size, report.SHA256)
	return nil
}
