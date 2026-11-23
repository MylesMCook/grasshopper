package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	piextension "github.com/MylesMCook/grasshopper/integrations/pi"
)

// Pi connects through the Grasshopper Pi package, or, without it, through one
// extension file the client writes in Pi's user folder. The extension runs the self-updated client, and
// the client rewrites the file when its template changes, so Pi follows
// client releases without a plugin step. A pi-off file in the managed folder
// keeps the client from writing it again after the owner removes it.

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
	line := strings.Replace(piextension.ConfigLine, `""`, string(quoted), 1)
	return []byte(strings.Replace(piextension.Extension, piextension.ConfigLine, line, 1))
}

// PiPackageInstalled reports whether Pi's user settings list the Grasshopper
// Pi package. The package then serves Pi, and the client stops writing its
// own copy of the extension so Pi does not load it twice.
func PiPackageInstalled(agentDir string) bool {
	data, err := os.ReadFile(filepath.Join(agentDir, "settings.json"))
	if err != nil {
		return false
	}
	var settings struct {
		Packages []json.RawMessage `json:"packages"`
	}
	if json.Unmarshal(data, &settings) != nil {
		return false
	}
	for _, raw := range settings.Packages {
		var source string
		if json.Unmarshal(raw, &source) != nil {
			var entry struct {
				Source string `json:"source"`
			}
			if json.Unmarshal(raw, &entry) != nil {
				continue
			}
			source = entry.Source
		}
		if isGrasshopperPackage(source) {
			return true
		}
	}
	return false
}

// isGrasshopperPackage matches the git sources Pi accepts for this
// repository, such as git:github.com/MylesMCook/grasshopper@v2.14.0 or
// https://github.com/MylesMCook/grasshopper.
func isGrasshopperPackage(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	for _, prefix := range []string{"git:", "https://", "http://", "ssh://", "git@"} {
		source = strings.TrimPrefix(source, prefix)
	}
	source = strings.Replace(source, "github.com:", "github.com/", 1)
	if at := strings.LastIndex(source, "@"); at > 0 {
		source = source[:at]
	}
	source = strings.TrimSuffix(strings.TrimSuffix(source, "/"), ".git")
	return source == "github.com/mylesmcook/grasshopper"
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
// turned off, and the file differs. It reports whether it wrote. A relative
// configuration path is made absolute first, because the extension is used
// from every folder Pi starts in.
func SyncPi(configPath, agentDir string) (bool, error) {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return false, err
	}
	if PiOff(configPath) {
		return false, nil
	}
	if info, err := os.Stat(agentDir); err != nil || !info.IsDir() {
		return false, nil
	}
	if PiPackageInstalled(agentDir) {
		if err := os.Remove(PiExtensionPath(agentDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return false, nil
	}
	path, want := PiExtensionPath(agentDir), PiExtension(configPath)
	if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, want) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := writeAtomic(path, want); err != nil {
		return false, err
	}
	// RemovePi writes pi-off before deleting the extension. A removal that
	// raced this write has set pi-off by now, so undo the write rather than
	// leave Pi connected after the owner removed it.
	if PiOff(configPath) {
		return false, os.Remove(path)
	}
	return true, nil
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
