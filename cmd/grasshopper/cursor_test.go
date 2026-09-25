package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCursorWiringPreservesOtherServersAndHooks(t *testing.T) {
	dir := t.TempDir()
	mcpPath, hooksPath := filepath.Join(dir, "mcp.json"), filepath.Join(dir, "hooks.json")
	if err := os.WriteFile(mcpPath, []byte(`{"mcpServers":{"other":{"command":"other-tool"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hooksPath, []byte(`{"version":1,"hooks":{"sessionStart":[{"command":"other-hook"}],"stop":[{"command":"stop-hook"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "client.json")
	binary := filepath.Join(dir, "bin", "grasshopper")
	if err := cursorWiring(dir, config, binary, true, false); err != nil {
		t.Fatal(err)
	}
	if err := cursorWiring(dir, config, binary, true, false); err != nil {
		t.Fatalf("idempotent install: %v", err)
	}
	mcp, err := readJSONObject(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	servers := mcp["mcpServers"].(map[string]any)
	if len(servers) != 2 || servers["other"] == nil || servers["grasshopper"] == nil {
		t.Fatalf("MCP merge: %+v", servers)
	}
	hooks, err := readJSONObject(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	events := hooks["hooks"].(map[string]any)
	if len(events["sessionStart"].([]any)) != 2 || events["stop"] == nil {
		t.Fatalf("hooks merge: %+v", events)
	}
	if err := cursorWiring(dir, config, binary, false, false); err != nil {
		t.Fatal(err)
	}
	mcp, _ = readJSONObject(mcpPath)
	servers = mcp["mcpServers"].(map[string]any)
	if len(servers) != 1 || servers["other"] == nil {
		t.Fatalf("MCP removal: %+v", servers)
	}
	hooks, _ = readJSONObject(hooksPath)
	events = hooks["hooks"].(map[string]any)
	if len(events["sessionStart"].([]any)) != 1 || events["stop"] == nil {
		t.Fatalf("hooks removal: %+v", events)
	}
	if _, err := json.Marshal(hooks); err != nil {
		t.Fatal(err)
	}
}

func TestCursorWiringRefusesConflictingServer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"grasshopper":{"command":"other"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := cursorWiring(dir, "/test/client.json", "/test/grasshopper", true, true); err == nil {
		t.Fatal("conflicting MCP entry replaced")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"mcpServers":{"grasshopper":{"command":"other"}}}` {
		t.Fatal("conflicting MCP entry changed")
	}
}

func TestCursorRemovalKeepsUnrelatedHookMention(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")
	data := []byte(`{"version":1,"hooks":{"sessionStart":[{"command":"echo grasshopper hook --harness cursor"}]}}`)
	if err := os.WriteFile(hooksPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cursorWiring(dir, filepath.Join(dir, "client.json"), filepath.Join(dir, "grasshopper"), false, false); err != nil {
		t.Fatal(err)
	}
	hooks, err := readJSONObject(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	list := hooks["hooks"].(map[string]any)["sessionStart"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["command"] != "echo grasshopper hook --harness cursor" {
		t.Fatal("removed an unrelated Cursor hook")
	}
}

func TestCursorCLIPermissionsPreserveOtherRulesAndRemoveOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli-config.json")
	original := []byte(`{"version":1,"editor":{"vimMode":true},"permissions":{"allow":["Shell(ls)","Mcp(other:search)"],"deny":["Shell(rm)"]},"model":{"name":"test"}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := cursorCLIPermissions(path, true, false); err != nil {
			t.Fatal(err)
		}
	}
	config, err := readJSONObject(path)
	if err != nil {
		t.Fatal(err)
	}
	permissions := config["permissions"].(map[string]any)
	allow := permissions["allow"].([]any)
	if len(allow) != 5 || !containsAny(allow, "Shell(ls)") || !containsAny(allow, "Mcp(other:search)") || containsAny(allow, "Mcp(grasshopper:store)") {
		t.Fatalf("unexpected read permissions: %v", allow)
	}
	if err := cursorCLIPermissions(path, false, false); err != nil {
		t.Fatal(err)
	}
	config, err = readJSONObject(path)
	if err != nil {
		t.Fatal(err)
	}
	permissions = config["permissions"].(map[string]any)
	if !sameJSON(permissions["allow"], []any{"Shell(ls)", "Mcp(other:search)"}) || !sameJSON(permissions["deny"], []any{"Shell(rm)"}) {
		t.Fatalf("other permissions changed: %v", permissions)
	}
	if !sameJSON(config["model"], map[string]any{"name": "test"}) || !sameJSON(config["editor"], map[string]any{"vimMode": true}) {
		t.Fatal("other Cursor CLI settings changed")
	}
}

func TestCursorProjectCLIPermissionsContainOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.json")
	if err := cursorCLIPermissions(path, true, false); err != nil {
		t.Fatal(err)
	}
	config, err := readJSONObject(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(config) != 1 || config["permissions"] == nil {
		t.Fatalf("project CLI config gained global settings: %v", config)
	}
}

func TestCursorCLIPermissionsRejectMalformedConfigWithoutChangingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli-config.json")
	original := []byte(`{"version":1,"permissions":{"allow":["Shell(ls)",12]}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := cursorCLIPermissions(path, true, false); err == nil {
		t.Fatal("malformed allowlist accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("malformed CLI config was changed")
	}
}
