// grasshopper-go-backup creates a consistent, no-overwrite SQLite snapshot.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/MylesMCook/grasshopper/internal/gomemory"
)

func prepareQuickstartBackup(dataDir string) (string, error) {
	if dataDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(base, "Grasshopper")
	}
	if !filepath.IsAbs(dataDir) {
		return "", errors.New("quickstart data-dir must be absolute")
	}
	info, err := os.Lstat(dataDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("quickstart data directory is missing or linked")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", errors.New("quickstart data directory must be private")
	}
	for _, name := range []string{"memory.db", "access-token"} {
		item, err := os.Lstat(filepath.Join(dataDir, name))
		if err != nil || !item.Mode().IsRegular() {
			return "", errors.New("quickstart state is incomplete; no backup made")
		}
	}
	backupDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", err
	}
	backupInfo, err := os.Lstat(backupDir)
	if err != nil || !backupInfo.IsDir() || backupInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("backup directory is not a private directory")
	}
	if runtime.GOOS != "windows" && backupInfo.Mode().Perm()&0077 != 0 {
		return "", errors.New("backup directory must be private")
	}
	nonce := make([]byte, 4)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	destination := filepath.Join(backupDir, "memory-"+time.Now().UTC().Format("20060102T150405Z")+"-"+hex.EncodeToString(nonce)+".db")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	copy, err := gomemory.OpenWritableCopy(ctx, filepath.Join(dataDir, "memory.db"), destination)
	if err != nil {
		return "", err
	}
	if err := copy.Close(); err != nil {
		return "", err
	}
	return destination, nil
}

func run() error {
	var source, destination, dataDir string
	var quickstart bool
	flag.StringVar(&source, "source", "", "existing Grasshopper database to back up")
	flag.StringVar(&destination, "dest", "", "new backup path; must not exist")
	flag.BoolVar(&quickstart, "quickstart", false, "back up and verify a local quickstart database before an update")
	flag.StringVar(&dataDir, "data-dir", "", "quickstart state directory")
	flag.Parse()
	if quickstart {
		if source != "" || destination != "" {
			return errors.New("quickstart backup cannot use source or dest flags")
		}
		path, err := prepareQuickstartBackup(dataDir)
		if err != nil {
			return err
		}
		fmt.Printf("Verified local snapshot: %s\nKeep an off-host backup too.\n", path)
		return nil
	}
	if dataDir != "" {
		return errors.New("data-dir requires quickstart")
	}
	if source == "" || destination == "" {
		return errors.New("source and new destination are required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	copy, err := gomemory.OpenWritableCopy(ctx, source, destination)
	if err != nil {
		return err
	}
	if err := copy.Close(); err != nil {
		return err
	}
	fmt.Println("Consistent backup ready. Verify it before changing the source.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Grasshopper backup:", err)
		os.Exit(1)
	}
}
