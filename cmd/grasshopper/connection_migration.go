package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

// migrateDeviceCredential stages only an already host-verified device credential.
// The caller must verify role and device before this helper. Originals remain
// untouched; setup performs the normal config/wiring transaction afterwards.
func migrateDeviceCredential(existing goclient.Config, canonical string) (string, error) {
	if _, err := os.Lstat(canonical); !errors.Is(err, os.ErrNotExist) {
		return "", connectProblem("conflicting_configuration", "A shared connection already exists or cannot be inspected. Keep both connections and inspect them before changing either.")
	}
	if _, err := os.Lstat(pendingPath(canonical)); !errors.Is(err, os.ErrNotExist) {
		return "", connectProblem("conflicting_configuration", "A shared approval already exists. Finish or inspect it before migration.")
	}
	if existing.TokenEnv != "" || existing.TokenFile == "" {
		return "", connectProblem("conflicting_configuration", "An explicit credential source is configured. Preserve it and inspect the connection before migration.")
	}
	// The caller validated this credential with Remote. Preserve its source
	// and copy only a small regular file.
	info, err := os.Lstat(existing.TokenFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return "", connectProblem("conflicting_configuration", "The legacy credential needs inspection before migration.")
	}
	data, err := os.ReadFile(existing.TokenFile)
	if err != nil {
		return "", err
	}
	target := filepath.Join(filepath.Dir(canonical), "device-token")
	if err := os.MkdirAll(filepath.Dir(canonical), 0700); err != nil {
		return "", err
	}
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return "", connectProblem("conflicting_configuration", "The shared credential is not a regular file. Inspect it before migration.")
		}
		// Reject links/nonprivate files through the regular remote credential loader.
		test := existing
		test.TokenFile = target
		if _, err := goclient.NewRemote(test); err != nil {
			return "", connectProblem("conflicting_configuration", "The shared credential needs inspection before migration.")
		}
		saved, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(data, saved) {
			return "", connectProblem("conflicting_configuration", "A different shared credential exists. Existing access was kept; inspect it before changing it.")
		}
		return target, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(target)
		return "", err
	}
	return target, nil
}
