package client

import (
	"bytes"
	"errors"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Pi has no plugin marketplace, so Grasshopper connects it with one extension
// file in Pi's user folder. The extension runs the self-updated client, and
// the client rewrites the file when its template changes, so Pi follows
// client releases without a plugin step. A pi-off file in the managed folder
// keeps the client from writing it again after the owner removes it.

//go:embed pi_extension.ts
var piExtensionTemplate string

// PiAgentDir is Pi's user configuration folder.
func PiAgentDir() (string, error) {
	if dir := os.Getenv("PI_CODING_AGENT_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi", "agent"), nil
}

// PiExtensionPath is where the Grasshopper extension lives in agentDir.
func PiExtensionPath(agentDir string) string {
	return filepath.Join(agentDir, "extensions", "grasshopper.ts")
}

// PiExtension is the extension for a client configuration path.
func PiExtension(configPath string) []byte {
	quoted, _ := json.Marshal(configPath)
	return []byte(strings.Replace(piExtensionTemplate, "__GRASSHOPPER_CONFIG__", string(quoted), 1))
}

func piOffPath(configPath string) string {
	return filepath.Join(ManagedDir(configPath), "pi-off")
}

// PiOff reports whether the owner turned Pi's connection off.
func PiOff(configPath string) bool {
	_, err := os.Lstat(piOffPath(configPath))
	return err == nil
}

// SyncPi writes the extension when Pi is installed, its connection is not
// turned off, and the file differs. It reports whether it wrote.
func SyncPi(configPath, agentDir string) (bool, error) {
	if PiOff(configPath) {
		return false, nil
	}
	if info, err := os.Stat(agentDir); err != nil || !info.IsDir() {
		return false, nil
	}
	path, want := PiExtensionPath(agentDir), PiExtension(configPath)
	if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, want) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	return true, writeAtomic(path, want)
}

// InstallPi connects Pi even when automatic wiring was turned off.
func InstallPi(configPath, agentDir string) error {
	if err := os.Remove(piOffPath(configPath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return err
	}
	_, err := SyncPi(configPath, agentDir)
	return err
}

// RemovePi deletes the extension and keeps the client from restoring it.
func RemovePi(configPath, agentDir string) error {
	if err := os.MkdirAll(ManagedDir(configPath), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(piOffPath(configPath), nil, 0o600); err != nil {
		return err
	}
	if err := os.Remove(PiExtensionPath(agentDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
