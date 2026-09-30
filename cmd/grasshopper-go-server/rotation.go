package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// rotateOwnerToken is an offline operation. A running handler retains its token
// hash until restart; the caller must stop every server using this directory.
func rotateOwnerToken(dir string, stopped bool) error {
	if !stopped {
		return errors.New("stop the server first, then use --rotate-token --server-stopped --data-dir; running credentials remain valid until restart")
	}
	if !filepath.IsAbs(dir) {
		return errors.New("rotation requires an absolute --data-dir")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("data-dir must be an existing unlinked directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("data-dir must be private (0700)")
	}
	db, err := os.Lstat(filepath.Join(dir, "memory.db"))
	if err != nil || !db.Mode().IsRegular() {
		return errors.New("data-dir must contain a regular memory.db; rotation does not create or repair databases")
	}
	path := filepath.Join(dir, "access-token")
	previous := path + ".previous"
	if _, err := os.Lstat(previous); !errors.Is(err, os.ErrNotExist) {
		return errors.New("access-token.previous already exists; preserve it elsewhere before another rotation")
	}
	var old []byte
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("access-token must be a regular unlinked file")
		}
		if _, err := readToken(path); err != nil {
			return fmt.Errorf("access-token: %w", err)
		}
		old, err = os.ReadFile(path)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".access-token-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := fmt.Fprintln(tmp, base64.RawURLEncoding.EncodeToString(secret)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if old != nil {
		rollback, err := os.OpenFile(previous, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := rollback.Write(old)
		syncErr := rollback.Sync()
		closeErr := rollback.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			os.Remove(previous)
			return err
		}
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace access-token (rollback retained): %w", err)
	}
	if runtime.GOOS != "windows" {
		directory, err := os.Open(dir)
		if err != nil {
			return fmt.Errorf("token replaced, but data-dir durability check failed: %w", err)
		}
		syncErr := directory.Sync()
		closeErr := directory.Close()
		if err := errors.Join(syncErr, closeErr); err != nil {
			return fmt.Errorf("token replaced, but data-dir sync failed: %w", err)
		}
	}
	return nil
}
