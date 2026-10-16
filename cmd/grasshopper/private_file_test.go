package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/client"
)

func TestPrivateFileRefusesToReplaceASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "dotfiles-mcp.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "mcp.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := privateFile(link, []byte(`{"changed":true}`)); err == nil {
		t.Fatal("privateFile replaced a symlink")
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the symlink is gone: %v", err)
	}
	if data, _ := os.ReadFile(target); string(data) != "{}\n" {
		t.Fatalf("the linked file changed: %q", data)
	}
}

func TestConfigureSavesTheMCPAddressForABareServerLink(t *testing.T) {
	root := t.TempDir()
	policy := filepath.Join(root, "source", "AGENTS.md")
	if err := os.Mkdir(filepath.Dir(policy), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("Canonical memory policy.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte(strings.Repeat("a", 40)), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "client", "client.json")
	if err := configure([]string{"--url", "https://memory.example.test", "--token-file", token, "--device", "test-mac", "--policy-file", policy, "--config", configPath}); err != nil {
		t.Fatal(err)
	}
	loaded, err := client.LoadConfig(configPath)
	if err != nil || loaded.URL != "https://memory.example.test/mcp" {
		t.Fatalf("saved address %q: %v", loaded.URL, err)
	}
}

func TestSetupLeavesADanglingCursorSymlinkInPlace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need extra privileges on Windows")
	}
	root, token, config, cursorDir := setupFixture(t)
	server := setupServer(t, true)
	if err := os.MkdirAll(cursorDir, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cursorDir, "mcp.json")
	if err := os.Symlink(filepath.Join(root, "dotfiles", "mcp.json"), link); err != nil {
		t.Fatal(err)
	}
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	err := setupClientWithRoot([]string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--agents", "cursor", "--config", config, "--cursor-dir", cursorDir}, root, run)
	if err == nil {
		t.Fatal("setup replaced a symlinked Cursor config")
	}
	if info, lerr := os.Lstat(link); lerr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("setup removed the symlink (setup error: %v): %v", err, lerr)
	}
}
