package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/MylesMCook/grasshopper/internal/goclient"
)

func TestConfigureUsesOnePolicyAndExistingToken(t *testing.T) {
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
	args := []string{"--url", "http://127.0.0.1:8106/mcp", "--token-file", token, "--device", "test-mac", "--policy-file", policy, "--config", configPath}
	if err := configure(args); err != nil {
		t.Fatal(err)
	}
	if err := configure(args); err != nil {
		t.Fatalf("idempotent configure: %v", err)
	}
	loaded, err := goclient.LoadConfig(configPath)
	if err != nil || loaded.TokenFile != token || loaded.Device != "test-mac" || loaded.PolicyPath != filepath.Join(root, "client", "AGENTS.md") {
		t.Fatalf("unexpected client config: %+v: %v", loaded, err)
	}
	for _, path := range []string{configPath, loaded.PolicyPath} {
		info, err := os.Stat(path)
		if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
			t.Fatalf("private file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "client", "token")); !os.IsNotExist(err) {
		t.Fatal("configure copied the token")
	}
	changed := append([]string(nil), args...)
	changed[5] = "other-mac"
	if err := configure(changed); err == nil {
		t.Fatal("configuration changed without --update")
	}
	if err := configure(append(changed, "--update")); err != nil {
		t.Fatalf("explicit update: %v", err)
	}
}

func TestConfigureRejectsUnsafeCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACLs are not represented by POSIX file modes")
	}
	root := t.TempDir()
	policy := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(policy, []byte("Policy.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte(strings.Repeat("a", 40)), 0644); err != nil {
		t.Fatal(err)
	}
	args := []string{"--url", "https://memory.example.com/mcp", "--token-file", token, "--device", "test-mac", "--policy-file", policy, "--config", filepath.Join(root, "client", "client.json")}
	if err := configure(args); err == nil {
		t.Fatal("world-readable token accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "client", "client.json")); !os.IsNotExist(err) {
		t.Fatal("invalid credentials created a client config")
	}
}

func TestConfigureRejectsInvalidCredential(t *testing.T) {
	root := t.TempDir()
	policy := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(policy, []byte("Policy.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte("too-short"), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "client", "client.json")
	args := []string{"--url", "https://memory.example.com/mcp", "--token-file", token, "--device", "test", "--policy-file", policy, "--config", configPath}
	if err := configure(args); err == nil {
		t.Fatal("invalid credential accepted")
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("invalid credential created a client config")
	}
}
