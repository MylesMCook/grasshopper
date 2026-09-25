package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeReadPermissionsPreserveSettingsAndRejectWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte(`{"permissions":{"allow":["Read"],"deny":["Bash(rm *)"]},"outputStyle":"plain"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := allowClaudeReads(path, false); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := readJSONObject(path)
	if err != nil {
		t.Fatal(err)
	}
	permissions := settings["permissions"].(map[string]any)
	allow := permissions["allow"].([]any)
	if len(allow) != 4 || !containsAny(allow, "Read") || !sameJSON(permissions["deny"], []any{"Bash(rm *)"}) || settings["outputStyle"] != "plain" {
		t.Fatalf("Claude settings not preserved: %v", settings)
	}
	for _, read := range claudeReadPermissions {
		if !containsAny(allow, read) {
			t.Fatalf("read rule missing: %s", read)
		}
	}
	for _, write := range []string{"mcp__plugin_grasshopper_grasshopper__store", "mcp__plugin_grasshopper_grasshopper__archive", "mcp__plugin_grasshopper_grasshopper__*"} {
		if containsAny(allow, write) {
			t.Fatalf("write rule granted: %s", write)
		}
	}
}

func TestClaudeReadPermissionsRejectMalformedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte(`{"permissions":{"allow":["Read",42]}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := allowClaudeReads(path, true); err == nil {
		t.Fatal("malformed Claude permissions accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("malformed Claude settings were changed")
	}
}
