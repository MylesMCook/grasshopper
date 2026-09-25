package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupFixture(t *testing.T) (string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	for _, path := range []string{"policy/AGENTS.md", "codex/.agents/plugins/marketplace.json", "claude/.claude-plugin/marketplace.json", "cursor/plugins/grasshopper/bin/grasshopper"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte(strings.Repeat("t", 40)), 0600); err != nil {
		t.Fatal(err)
	}
	return root, token, filepath.Join(root, "client", "client.json"), filepath.Join(root, "cursor-user")
}

func setupServer(t *testing.T, allowed bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 40) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "initialize":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-03-26"}}`))
		case "tools/call":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"structuredContent":{"records":[]}}}`))
		default:
			t.Errorf("unexpected method %s", request.Method)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestSetupRejectsBadAuthenticationBeforeChanges(t *testing.T) {
	root, token, config, cursorDir := setupFixture(t)
	server := setupServer(t, false)
	called := false
	run := func(name string, args ...string) ([]byte, error) { called = true; return nil, nil }
	err := setupClientWithRoot([]string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--agents", "cursor", "--config", config, "--cursor-dir", cursorDir}, root, run)
	if err == nil || called {
		t.Fatalf("bad authentication changed agent setup: %v", err)
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("bad authentication wrote client config")
	}
	if _, err := os.Stat(cursorDir); !os.IsNotExist(err) {
		t.Fatal("bad authentication wrote Cursor config")
	}
}

func TestSetupRejectsUnknownAgentBeforeChanges(t *testing.T) {
	root, token, config, cursorDir := setupFixture(t)
	run := func(string, ...string) ([]byte, error) { t.Fatal("native command ran"); return nil, nil }
	err := setupClientWithRoot([]string{"--url", "http://127.0.0.1:1/mcp", "--token-file", token, "--agents", "other", "--config", config, "--cursor-dir", cursorDir}, root, run)
	if err == nil {
		t.Fatal("unknown agent accepted")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("unknown agent wrote config")
	}
}

func TestSetupRejectsConflictingCursorEntryBeforeChanges(t *testing.T) {
	root, token, config, cursorDir := setupFixture(t)
	server := setupServer(t, true)
	if err := os.MkdirAll(cursorDir, 0700); err != nil {
		t.Fatal(err)
	}
	entry := []byte(`{"mcpServers":{"grasshopper":{"command":"other-tool"}}}`)
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), entry, 0600); err != nil {
		t.Fatal(err)
	}
	run := func(string, ...string) ([]byte, error) { t.Fatal("native command ran"); return nil, nil }
	err := setupClientWithRoot([]string{"--url", server.URL + "/mcp", "--token-file", token, "--agents", "cursor", "--config", config, "--cursor-dir", cursorDir}, root, run)
	if err == nil {
		t.Fatal("conflicting Cursor entry accepted")
	}
	if got, err := os.ReadFile(filepath.Join(cursorDir, "mcp.json")); err != nil || string(got) != string(entry) {
		t.Fatal("conflicting Cursor entry changed")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatal("Cursor conflict wrote client config")
	}
}

func TestSetupConnectsCursorWithoutAnotherMemoryStore(t *testing.T) {
	root, token, config, cursorDir := setupFixture(t)
	server := setupServer(t, true)
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		commands = append(commands, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	args := []string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--agents", "cursor", "--config", config, "--cursor-dir", cursorDir}
	if err := setupClientWithRoot(args, root, run); err != nil {
		t.Fatal(err)
	}
	if err := setupClientWithRoot(args, root, run); err != nil {
		t.Fatalf("repeat setup: %v", err)
	}
	for _, path := range []string{config, filepath.Join(filepath.Dir(config), "AGENTS.md"), filepath.Join(cursorDir, "mcp.json"), filepath.Join(cursorDir, "hooks.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(config), "token")); !os.IsNotExist(err) {
		t.Fatal("setup copied the token")
	}
	mcp, err := readJSONObject(filepath.Join(cursorDir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	servers := mcp["mcpServers"].(map[string]any)
	if len(servers) != 1 || servers["grasshopper"] == nil {
		t.Fatalf("unexpected MCP wiring: %v", servers)
	}
	_ = commands
}

func TestSetupInstallsSelectedNativePlugins(t *testing.T) {
	root, token, config, _ := setupFixture(t)
	server := setupServer(t, true)
	var commands []string
	run := func(name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "codex plugin marketplace list --json":
			return []byte(`{"marketplaces":[]}`), nil
		case "codex plugin list --json":
			return []byte(`{"installed":[]}`), nil
		case "claude plugin marketplace list --json", "claude plugin list --json":
			return []byte(`[]`), nil
		default:
			return nil, nil
		}
	}
	args := []string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--agents", "codex,claude", "--config", config}
	if err := setupClientWithRoot(args, root, run); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"codex plugin marketplace add " + filepath.Join(root, "codex"),
		"codex plugin add grasshopper@grasshopper-local",
		"claude plugin marketplace add " + filepath.Join(root, "claude"),
		"claude plugin install grasshopper@grasshopper-local",
	} {
		if !containsString(commands, expected) {
			t.Errorf("missing native command %q: %v", expected, commands)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestSetupUpdateReplacesOnlyGrasshopperCursorWiring(t *testing.T) {
	oldRoot, token, config, cursorDir := setupFixture(t)
	server := setupServer(t, true)
	run := func(string, ...string) ([]byte, error) { return nil, nil }
	args := []string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--agents", "cursor", "--config", config, "--cursor-dir", cursorDir}
	if err := setupClientWithRoot(args, oldRoot, run); err != nil {
		t.Fatal(err)
	}
	newRoot := t.TempDir()
	for _, path := range []string{"policy/AGENTS.md", "cursor/plugins/grasshopper/bin/grasshopper"} {
		full := filepath.Join(newRoot, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("new package"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := setupClientWithRoot(args, newRoot, run); err == nil {
		t.Fatal("replaced existing Cursor package without --update")
	}
	if err := setupClientWithRoot(append(args, "--update"), newRoot, run); err != nil {
		t.Fatal(err)
	}
	mcp, err := readJSONObject(filepath.Join(cursorDir, "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	serverEntry := mcp["mcpServers"].(map[string]any)["grasshopper"].(map[string]any)
	if serverEntry["command"] != filepath.Join(newRoot, "cursor", "plugins", "grasshopper", "bin", "grasshopper") {
		t.Fatalf("Cursor still uses old binary: %v", serverEntry)
	}
	if _, err := os.Stat(filepath.Join(oldRoot, "cursor", "plugins", "grasshopper", "bin", "grasshopper")); err != nil {
		t.Fatal("old package was removed")
	}
}

func TestSetupUpdateRestoresConfigAndPluginOnFailure(t *testing.T) {
	oldRoot, token, config, _ := setupFixture(t)
	server := setupServer(t, true)
	oldMarketplace := filepath.Join(oldRoot, "codex")
	newRoot := t.TempDir()
	for _, path := range []string{"policy/AGENTS.md", "codex/.agents/plugins/marketplace.json"} {
		full := filepath.Join(newRoot, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("new package"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	oldConfig := []byte("existing client config\n")
	if err := os.WriteFile(config, oldConfig, 0600); err != nil {
		t.Fatal(err)
	}
	var commands []string
	adds := 0
	run := func(name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "codex plugin marketplace list --json":
			return []byte(`{"marketplaces":[{"name":"grasshopper-local","root":` + jsonString(oldMarketplace) + `}]}`), nil
		case "codex plugin list --json":
			return []byte(`{"installed":[{"pluginId":"grasshopper@grasshopper-local"}]}`), nil
		case "codex plugin add grasshopper@grasshopper-local":
			adds++
			if adds == 1 {
				return nil, errors.New("synthetic install failure")
			}
		}
		return nil, nil
	}
	args := []string{"--url", server.URL + "/mcp", "--token-file", token, "--device", "test", "--agents", "codex", "--config", config, "--update"}
	if err := setupClientWithRoot(args, newRoot, run); err == nil {
		t.Fatal("failed replacement reported success")
	}
	gotConfig, err := os.ReadFile(config)
	if err != nil || string(gotConfig) != string(oldConfig) {
		t.Fatalf("client config was not restored: %q %v", gotConfig, err)
	}
	if !containsString(commands, "codex plugin marketplace add "+oldMarketplace) || adds != 2 {
		t.Fatalf("old plugin was not restored: %v", commands)
	}
}

func jsonString(value string) string { data, _ := json.Marshal(value); return string(data) }
