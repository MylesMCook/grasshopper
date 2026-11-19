package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncPiWritesTheExtensionOnlyWhenPiIsInstalled(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, `grass "hopper"`, "client.json")
	agentDir := filepath.Join(root, "pi", "agent")

	if wrote, err := SyncPi(config, agentDir); err != nil || wrote {
		t.Fatalf("without Pi nothing is written, got wrote=%v err=%v", wrote, err)
	}
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if wrote, err := SyncPi(config, agentDir); err != nil || !wrote {
		t.Fatalf("installed Pi gets the extension, got wrote=%v err=%v", wrote, err)
	}
	written, err := os.ReadFile(PiExtensionPath(agentDir))
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(config)
	if !strings.Contains(string(written), "const config: string = "+string(quoted)+";") || strings.Contains(string(written), "__GRASSHOPPER_CONFIG__") {
		t.Fatalf("extension must carry the quoted config path:\n%s", written)
	}
	if wrote, err := SyncPi(config, agentDir); err != nil || wrote {
		t.Fatalf("an unchanged extension is not rewritten, got wrote=%v err=%v", wrote, err)
	}
	if err := os.WriteFile(PiExtensionPath(agentDir), []byte("// old template\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if wrote, err := SyncPi(config, agentDir); err != nil || !wrote {
		t.Fatalf("a changed template is rewritten, got wrote=%v err=%v", wrote, err)
	}
}

func TestRemovePiKeepsSyncFromRestoringIt(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "grasshopper", "client.json")
	agentDir := filepath.Join(root, "pi", "agent")
	if err := InstallPi(config, agentDir); err != nil {
		t.Fatal(err)
	}
	if err := RemovePi(config, agentDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(PiExtensionPath(agentDir)); !os.IsNotExist(err) {
		t.Fatalf("remove deletes the extension, got %v", err)
	}
	if wrote, err := SyncPi(config, agentDir); err != nil || wrote || !PiOff(config) {
		t.Fatalf("sync must respect removal, got wrote=%v err=%v off=%v", wrote, err, PiOff(config))
	}
	if err := InstallPi(config, agentDir); err != nil || PiOff(config) {
		t.Fatalf("install turns Pi back on, got err=%v off=%v", err, PiOff(config))
	}
	if _, err := os.Stat(PiExtensionPath(agentDir)); err != nil {
		t.Fatalf("install writes the extension again: %v", err)
	}
}

// The extension is used from every folder Pi starts in, so a relative
// configuration path must be written as an absolute one.
func TestPiExtensionUsesAnAbsoluteConfigPath(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, "pi", "agent")
	t.Chdir(root)
	if err := InstallPi(filepath.Join("settings", "client.json"), agentDir); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(PiExtensionPath(agentDir))
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(filepath.Join(root, "settings", "client.json"))
	if !strings.Contains(string(written), "const config: string = "+string(quoted)+";") {
		t.Fatalf("extension must carry the absolute config path:\n%s", written)
	}
}
